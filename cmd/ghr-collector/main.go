package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/google/go-github/v32/github"
	statspb "github.com/haya14busa/github-release-stats/proto/releasestats"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/encoding/protojson"
)

type option struct {
	owner   string
	repo    string
	basedir string
}

func registerFlags(opts *option) {
	flag.StringVar(&opts.owner, "owner", "", "Repository owner")
	flag.StringVar(&opts.repo, "repo", "", "Repository name")
	flag.StringVar(&opts.basedir, "basedir", "./data", "Base dir of stats data")
}

func main() {
	var opt option
	registerFlags(&opt)
	flag.Parse()
	if err := run(opt); err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
}

func run(opt option) error {
	if err := validateOption(opt); err != nil {
		return err
	}
	ghtoken, err := nonEmptyEnv("GITHUB_API_TOKEN")
	if err != nil {
		return err
	}
	ctx := context.Background()
	now := time.Now()

	// Load existing stats.
	stats, err := existingStats(opt)
	if err != nil {
		return err
	}

	// Fetch latest releases stats from GitHub.
	cil := githubClient(ctx, ghtoken)
	releases, err := listAllReleases(
		ctx, cil, opt.owner, opt.repo, github.ListOptions{PerPage: 100})
	if err != nil {
		return fmt.Errorf("failed to get releases: %v", err)
	}

	// Update stats.
	stats.History = append(stats.History, convertReleases(now.Unix(), releases))
	updateSummaryStats(stats)

	// Write stats.
	if err := os.MkdirAll(repoDataPath(opt), os.ModePerm); err != nil {
		return err
	}
	statsb, err := protojson.MarshalOptions{
		Multiline: true,
	}.Marshal(stats)
	if err := ioutil.WriteFile(statsDataPath(opt), statsb, os.ModePerm); err != nil {
		return err
	}
	if err := writeShieldEndpoints(stats, opt); err != nil {
		return err
	}
	return nil
}

func validateOption(opt option) error {
	if opt.owner == "" {
		return errors.New("-owner is empty")
	}
	if opt.repo == "" {
		return errors.New("-repo is empty")
	}
	return nil
}

func githubClient(ctx context.Context, token string) *github.Client {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{})
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

func nonEmptyEnv(env string) (string, error) {
	v := os.Getenv(env)
	if v == "" {
		return "", fmt.Errorf("environment variable $%v is not set", env)
	}
	return v, nil
}

func listAllReleases(ctx context.Context, cli *github.Client,
	owner, repo string, listOpts github.ListOptions) ([]*github.RepositoryRelease, error) {
	releases, resp, err := cli.Repositories.ListReleases(ctx, owner, repo, &listOpts)
	if err != nil {
		return nil, err
	}
	if resp.NextPage == 0 {
		return releases, nil
	}
	newOpts := listOpts
	newOpts.Page = resp.NextPage
	rest, err := listAllReleases(ctx, cli, owner, repo, newOpts)
	if err != nil {
		return nil, err
	}
	return append(releases, rest...), nil
}

func existingStats(opt option) (*statspb.ReleaseStats, error) {
	stats := &statspb.ReleaseStats{}
	path := statsDataPath(opt)
	if !fileExists(path) {
		// Return empty ReleaseStats with repo data.
		stats.Repo = &statspb.GitHubRepository{
			Owner: opt.owner,
			Repo:  opt.repo,
		}
		return stats, nil
	}
	// Load existing ReleaseStats.
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", path, err)
	}
	if err := protojson.Unmarshal(b, stats); err != nil {
		return nil, err
	}
	return stats, nil
}

func repoDataPath(opt option) string {
	return path.Join(opt.basedir, opt.owner, opt.repo)
}

func statsDataPath(opt option) string {
	return path.Join(repoDataPath(opt), "stats.json")
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func convertReleases(timestampSec int64, releases []*github.RepositoryRelease) *statspb.ReleaseStatsHistory {
	stats := &statspb.ReleaseStatsHistory{
		TimestampSeconds: timestampSec,
	}
	var total int64
	for _, r := range releases {
		statsr := convertRelease(r)
		stats.Release = append(stats.Release, statsr)
		total += statsr.GetTotalDownloadCount()
	}
	stats.TotalDownloadCount = total
	return stats
}

func convertRelease(r *github.RepositoryRelease) *statspb.Release {
	statsr := &statspb.Release{
		Id:      r.GetID(),
		TagName: r.GetTagName(),
	}
	var total int64
	for _, a := range r.Assets {
		statsa := convertAsset(a)
		statsr.Assets = append(statsr.Assets, statsa)
		total += statsa.GetDownloadCount()
	}
	statsr.TotalDownloadCount = total
	return statsr
}

func convertAsset(a *github.ReleaseAsset) *statspb.Asset {
	statsa := &statspb.Asset{
		Id:            a.GetID(),
		Name:          a.GetName(),
		DownloadCount: int64(a.GetDownloadCount()),
	}
	return statsa
}

const (
	timeBuffer  = time.Hour
	timeDaily   = 24*time.Hour*1 + timeBuffer
	timeWeekly  = 24*time.Hour*7 + timeBuffer
	timeMonthly = 24*time.Hour*30 + timeBuffer
)

func updateSummaryStats(stats *statspb.ReleaseStats) {
	if len(stats.History) == 0 {
		return
	}
	latest := stats.History[len(stats.History)-1]
	latestTime := time.Unix(latest.TimestampSeconds, 0)
	var (
		dailyStart   = latest
		weeklyStart  = latest
		monthlyStart = latest
	)
	for i := len(stats.History) - 2; i >= 0; i-- {
		stats := stats.History[i]
		duration := latestTime.Sub(time.Unix(stats.TimestampSeconds, 0))
		if duration >= timeMonthly {
			break
		}
		switch {
		case duration < timeDaily:
			dailyStart = stats
			fallthrough
		case duration < timeWeekly:
			weeklyStart = stats
			fallthrough
		case duration < timeMonthly:
			monthlyStart = stats
		}
	}
	stats.Summary = &statspb.ReleaseStats_StatsSummary{
		LatestTotalDownloads:  latest.TotalDownloadCount,
		DailyTotalDownloads:   latest.TotalDownloadCount - dailyStart.TotalDownloadCount,
		WeeklyTotalDownloads:  latest.TotalDownloadCount - weeklyStart.TotalDownloadCount,
		MonthlyTotalDownloads: latest.TotalDownloadCount - monthlyStart.TotalDownloadCount,
	}
}

// https://shields.io/endpoint
type shieldsResponse struct {
	SchemaVersion int    `json:"schemaVersion,omitempty"`
	Label         string `json:"label,omitempty"`
	Message       string `json:"message,omitempty"`
	Color         string `json:"color,omitempty"`
	NamedLogo     string `json:"namedLogo,omitempty"`
}

func writeShieldEndpoints(stats *statspb.ReleaseStats, opt option) error {
	writeShieldEndpoint(stats.GetSummary().GetLatestTotalDownloads(), "total.json", "", opt)
	writeShieldEndpoint(stats.GetSummary().GetDailyTotalDownloads(), "daily.json", "/day", opt)
	writeShieldEndpoint(stats.GetSummary().GetWeeklyTotalDownloads(), "weekly.json", "/week", opt)
	writeShieldEndpoint(stats.GetSummary().GetMonthlyTotalDownloads(), "monthly.json", "/month", opt)
	return nil
}

func writeShieldEndpoint(value int64, fileName string, suffix string, opt option) error {
	shieldsPath := path.Join(repoDataPath(opt), "shieldsio")
	if err := os.MkdirAll(shieldsPath, os.ModePerm); err != nil {
		return err
	}
	fpath := path.Join(shieldsPath, fileName)
	shields := shieldsResponse{
		SchemaVersion: 1,
		Label:         "downloads",
		NamedLogo:     "github",
		Color:         "brightgreen",
		Message:       readableNum(value, 1) + suffix,
	}
	b, err := json.Marshal(shields)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(fpath, b, os.ModePerm)
}

func readableNum(input int64, decimals int) string {
	value, prefix := humanize.ComputeSI(float64(input))
	return humanize.FtoaWithDigits(value, decimals) + prefix
}

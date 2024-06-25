const fs = require('fs');

const repoArg = process.argv[2];

if (!repoArg || !repoArg.includes('/')) {
  console.error('Usage: node script.js <owner/repo>');
  process.exit(1);
}

const [owner, repo] = repoArg.split('/');

function loadData(owner, repo) {
  try {
    const jsonData = JSON.parse(fs.readFileSync(`data/${owner}/${repo}/stats.json`, 'utf8'));
    return jsonData.history.map(item => ({
      date: new Date(item.timestampSeconds * 1000),
      downloads: item.totalDownloadCount
    }));
  } catch (error) {
    console.error('Error loading data:', error);
    return [];
  }
}

function formatNumber(num) {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
  return num.toString();
}

function createSVGChart(data, owner, repo, mode) {
  const width = 800;
  const height = 400;
  const margin = { top: 40, right: 70, left: 40, bottom: 60 };
  const chartWidth = width - margin.left - margin.right;
  const chartHeight = height - margin.top - margin.bottom;

  const maxDownloads = Math.max(...data.map(d => d.downloads));
  const minDate = new Date(Math.min(...data.map(d => d.date)));
  const maxDate = new Date(Math.max(...data.map(d => d.date)));

  const xScale = (date) => margin.left + ((date - minDate) / (maxDate - minDate)) * chartWidth;
  const yScale = (value) => margin.top + chartHeight - (value / maxDownloads) * chartHeight;

  const yTicks = [0, maxDownloads / 4, maxDownloads / 2, (maxDownloads * 3) / 4, maxDownloads];

  const xTicks = [];
  let currentDate = new Date(minDate);
  while (currentDate <= maxDate) {
    xTicks.push(new Date(currentDate));
    currentDate.setMonth(currentDate.getMonth() + 6);
  }

  const points = data.map(d => `${xScale(d.date)},${yScale(d.downloads)}`).join(' ');

  const colors = mode === 'dark' ? {
    bg: '#1a1a1a',
    text: '#ffffff',
    grid: '#333333',
    axis: '#999999',
    line: '#bb86fc'
  } : {
    bg: '#ffffff',
    text: '#333333',
    grid: '#dddddd',
    axis: '#666666',
    line: '#8884d8'
  };

  const svgContent = `
    <svg width="${width}" height="${height}" xmlns="http://www.w3.org/2000/svg">
      <style>
        .chart-bg { fill: ${colors.bg}; }
        .chart-line { fill: none; stroke: ${colors.line}; stroke-width: 1.5; }
        .axis { stroke: ${colors.axis}; stroke-width: 2; }
        .grid { stroke: ${colors.grid}; stroke-dasharray: 2,2; }
        .axis-label { font-family: Arial, sans-serif; font-size: 12px; fill: ${colors.text}; }
        .title { font-family: Arial, sans-serif; font-size: 16px; font-weight: bold; fill: ${colors.text}; }
      </style>

      <rect width="${width}" height="${height}" class="chart-bg" />
      <text x="${width / 2}" y="20" text-anchor="middle" class="title">${owner}/${repo} Release Stats: Total Downloads</text>

      <!-- Y axis (right) -->
      <line x1="${width - margin.right}" y1="${margin.top}" x2="${width - margin.right}" y2="${height - margin.bottom}" class="axis" />
      ${yTicks.map(tick => {
        const y = yScale(tick);
        return `
          <line x1="${margin.left}" y1="${y}" x2="${width - margin.right}" y2="${y}" class="grid" />
          <line x1="${width - margin.right}" y1="${y}" x2="${width - margin.right + 5}" y2="${y}" class="axis" />
          <text x="${width - margin.right + 10}" y="${y}" dy=".32em" text-anchor="start" class="axis-label">${formatNumber(tick)}</text>
        `;
      }).join('')}
      <text x="${width - margin.right + 55}" y="${height / 2}" transform="rotate(90 ${width - margin.right + 55} ${height / 2})" text-anchor="middle" class="axis-label">Total Downloads</text>

      <!-- X axis -->
      <line x1="${margin.left}" y1="${height - margin.bottom}" x2="${width - margin.right}" y2="${height - margin.bottom}" class="axis" />
      ${xTicks.map(date => {
        const x = xScale(date);
        return `
          <line x1="${x}" y1="${margin.top}" x2="${x}" y2="${height - margin.bottom}" class="grid" />
          <line x1="${x}" y1="${height - margin.bottom}" x2="${x}" y2="${height - margin.bottom + 5}" class="axis" />
          <text x="${x}" y="${height - margin.bottom + 20}" text-anchor="middle" class="axis-label">${date.toISOString().split('T')[0]}</text>
        `;
      }).join('')}
      <text x="${width / 2}" y="${height - 10}" text-anchor="middle" class="axis-label">Date</text>

      <!-- Data line -->
      <polyline points="${points}" class="chart-line" />

      <!-- Data points -->
      ${data.map(d => `
        <circle cx="${xScale(d.date)}" cy="${yScale(d.downloads)}" r="2" fill="${colors.line}" />
      `).join('')}
    </svg>
  `;

  return svgContent;
}

const data = loadData(owner, repo);
const lightModeSVG = createSVGChart(data, owner, repo, 'light');
const darkModeSVG = createSVGChart(data, owner, repo, 'dark');

fs.writeFileSync(`data/${owner}/${repo}/release_stats_chart_light.svg`, lightModeSVG);
fs.writeFileSync(`data/${owner}/${repo}/release_stats_chart_dark.svg`, darkModeSVG);

console.log(`Light mode SVG file has been generated: data/${owner}/${repo}/release_stats_chart_light.svg`);
console.log(`Dark mode SVG file has been generated: data/${owner}/${repo}/release_stats_chart_dark.svg`);


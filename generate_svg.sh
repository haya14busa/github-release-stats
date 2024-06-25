#!/bin/bash
find data | grep stats.json | sed 's|data/||; s|/stats.json||' | xargs -I {} node generate_svg.js {}


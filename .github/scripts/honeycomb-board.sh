#!/usr/bin/env bash
# Creates the "Verity Patch Pipeline" board in Honeycomb against the verity-ci dataset.
#
# Honeycomb's modern Boards API requires queries to be created via /1/queries first
# (no inline query support), so this script does the full 3-step flow:
# create queries → create query_annotations → create board.
#
# Requires: HONEYCOMB_API_KEY = a CONFIGURATION key (NOT an ingest key).
# Generate one at: https://ui.honeycomb.io/<team>/environments/<env>/api_keys
# The key needs at minimum: Manage Queries, Manage Public Boards.
#
# Re-running creates a new board each time (Honeycomb has no upsert). To update,
# delete the existing board in the UI and re-run.

set -euo pipefail

: "${HONEYCOMB_API_KEY:?Set HONEYCOMB_API_KEY to a configuration key (Manage Queries + Manage Public Boards)}"
DATASET="${HONEYCOMB_DATASET:-verity-ci}"
BASE="${HONEYCOMB_BASE_URL:-https://api.honeycomb.io}"
H=(-H "X-Honeycomb-Team: ${HONEYCOMB_API_KEY}" -H "Content-Type: application/json")

mkquery() {
  curl -fsS -X POST "${BASE}/1/queries/${DATASET}" "${H[@]}" -d "$1" | jq -r '.id'
}

mkannot() {
  curl -fsS -X POST "${BASE}/1/query_annotations/${DATASET}" "${H[@]}" \
    -d "$(jq -nc --arg q "$1" --arg n "$2" --arg d "$3" '{query_id:$q, name:$n, description:$d}')" \
    | jq -r '.id'
}

echo ">>> Q1: Patch duration p50/p95 HEATMAP"
Q1=$(mkquery '{
  "calculations":[
    {"op":"HEATMAP","column":"copa_duration_seconds"},
    {"op":"P50","column":"copa_duration_seconds"},
    {"op":"P95","column":"copa_duration_seconds"}
  ],
  "filters":[{"column":"name","op":"=","value":"patch-image.matrix"}],
  "time_range":604800
}')
A1=$(mkannot "$Q1" "Patch Duration p50/p95" "Copa patch wall-clock heatmap with p50/p95 overlays (matrix spans, 7d)")

echo ">>> Q2: CVE Burndown"
Q2=$(mkquery '{
  "calculations":[
    {"op":"SUM","column":"cve_before"},
    {"op":"SUM","column":"cve_after"}
  ],
  "filters":[{"column":"name","op":"=","value":"patch-image.matrix"}],
  "time_range":604800
}')
A2=$(mkannot "$Q2" "CVE Burndown" "Total CVEs before vs after across all patches over time")

echo ">>> Q3: Slowest Images"
Q3=$(mkquery '{
  "calculations":[{"op":"AVG","column":"copa_duration_seconds"}],
  "filters":[{"column":"name","op":"=","value":"patch-image.matrix"}],
  "breakdowns":["image"],
  "orders":[{"op":"AVG","column":"copa_duration_seconds","order":"descending"}],
  "limit":10,
  "time_range":604800
}')
A3=$(mkannot "$Q3" "Slowest Images" "Top 10 images by AVG(copa_duration_seconds)")

echo ">>> Q4: Failures by Image"
Q4=$(mkquery '{
  "calculations":[{"op":"COUNT"}],
  "filters":[
    {"column":"name","op":"=","value":"patch-image.matrix"},
    {"column":"copa_exit","op":"!=","value":0}
  ],
  "breakdowns":["image"],
  "time_range":604800
}')
A4=$(mkannot "$Q4" "Failures by Image" "Count of failed Copa patches grouped by image (copa_exit != 0)")

echo ">>> Q5: Patches by Platform"
Q5=$(mkquery '{
  "calculations":[{"op":"COUNT"}],
  "filters":[{"column":"name","op":"=","value":"patch-image.matrix"}],
  "breakdowns":["platform"],
  "time_range":604800
}')
A5=$(mkannot "$Q5" "Patches by Platform" "amd64 vs arm64 patch volume")

echo ">>> Q6: Residual CVEs by Image"
Q6=$(mkquery '{
  "calculations":[{"op":"AVG","column":"post_scan_vuln_count"}],
  "filters":[{"column":"name","op":"=","value":"patch-image.finalize"}],
  "breakdowns":["image"],
  "orders":[{"op":"AVG","column":"post_scan_vuln_count","order":"descending"}],
  "limit":25,
  "time_range":604800
}')
A6=$(mkannot "$Q6" "Residual CVEs by Image" "AVG post_scan_vuln_count after patch (finalize spans)")

echo ">>> Creating board"
BOARD_PAYLOAD=$(jq -nc \
  --arg q1 "$Q1" --arg a1 "$A1" \
  --arg q2 "$Q2" --arg a2 "$A2" \
  --arg q3 "$Q3" --arg a3 "$A3" \
  --arg q4 "$Q4" --arg a4 "$A4" \
  --arg q5 "$Q5" --arg a5 "$A5" \
  --arg q6 "$Q6" --arg a6 "$A6" '
{
  name: "Verity Patch Pipeline",
  description: "CI observability for Copa patch runs — Patch Lag SLO, CVE Burndown, Failure Rate. Auto-generated from .github/scripts/honeycomb-board.sh.",
  type: "flexible",
  column_layout: "multi",
  tags: [{key:"team",value:"verity"},{key:"owner",value:"platform"}],
  panels: [
    {type:"text",
     position:{x_coordinate:0,y_coordinate:0,width:12,height:2},
     text_panel:{content:"# Verity Patch Pipeline\n\n**Patch Lag SLO** · **CVE Burndown** · **Failure Rate** · **Platform Volume**\n\nScoped to `service.name = verity-ci`. Auto-generated from `.github/scripts/honeycomb-board.sh`."}},
    {type:"query",
     position:{x_coordinate:0,y_coordinate:2,width:12,height:6},
     query_panel:{query_id:$q1, query_annotation_id:$a1, query_style:"graph"}},
    {type:"query",
     position:{x_coordinate:0,y_coordinate:8,width:6,height:5},
     query_panel:{query_id:$q2, query_annotation_id:$a2, query_style:"graph"}},
    {type:"query",
     position:{x_coordinate:6,y_coordinate:8,width:6,height:5},
     query_panel:{query_id:$q4, query_annotation_id:$a4, query_style:"combo"}},
    {type:"query",
     position:{x_coordinate:0,y_coordinate:13,width:6,height:5},
     query_panel:{query_id:$q5, query_annotation_id:$a5, query_style:"combo"}},
    {type:"query",
     position:{x_coordinate:6,y_coordinate:13,width:6,height:5},
     query_panel:{query_id:$q3, query_annotation_id:$a3, query_style:"table"}},
    {type:"query",
     position:{x_coordinate:0,y_coordinate:18,width:12,height:5},
     query_panel:{query_id:$q6, query_annotation_id:$a6, query_style:"table"}}
  ]
}')

curl -fsS -X POST "${BASE}/1/boards" "${H[@]}" -d "${BOARD_PAYLOAD}" | jq '{id, name, links}'

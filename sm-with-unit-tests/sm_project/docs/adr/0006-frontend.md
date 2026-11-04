# ADR-0006: Single served HTML dashboard
Status: Accepted.

## Context
Challenge mandates one HTML file served by the system with searchable devices, sensor selection and 5m–1y chart.
## Decision
Single HTML file with inline JS/CSS and ECharts from a vendored resource embedded or inlined for fully offline launch; common Y axis 0..100; no frontend framework.
## Alternatives
React/Vue (more build overhead), manual Canvas (more custom code).
## Consequences
Simple deployment and runtime; organize JS in logical sections and add browser smoke coverage to avoid maintainability regression.

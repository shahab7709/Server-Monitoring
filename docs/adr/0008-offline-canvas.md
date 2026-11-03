# ADR-0008: Fully offline HTML Canvas renderer
Status: Accepted for implementation; supersedes library example in ADR-0006.

## Context
The UI must be a single HTML file served by Go; dependency downloads from a CDN could break deterministic offline Docker start.

## Decision
Use an embedded single-file HTML/CSS/JS with a small Canvas time-series renderer, dynamic searchable devices and explicit gaps. Show avg lines and min/max whiskers with a shared fixed 0–100 Y axis. Request at most 1500 buckets.

## Trade-offs
No third-party chart bundle or frontend build; lower maintenance overhead for small UI, but custom accessibility, zoom/tooltip support, and visual regression testing would be necessary at production scale. For expanded UI, vendor a chart library into the binary and keep HTML entry point single-file.

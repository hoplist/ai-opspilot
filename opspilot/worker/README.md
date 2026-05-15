# opspilot-worker

Async worker jobs for OpsPilot.

Initial jobs:

- `baseline-job`
- `health-snapshot-job`
- `incident-correlation-job`
- `backup-verify-ingest-job`
- `report-job`
- AI summary jobs

Workers can use Python first because they are async, analysis-heavy, and easier
to iterate independently from the online API.

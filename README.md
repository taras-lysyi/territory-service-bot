# Territory Service Bot

Telegram bot for managing territory assignments in congregations of Jehovah's Witnesses in Ukraine. A personal volunteer project.

![UI overview: registration, territory requests, administrator approvals, notes and returns, map uploads, and reports.](docs/ui-preview.png)

## Features

- Request and return territories through Telegram
- Admin approval for membership and territory requests
- Territory maps, documents and notes
- Optional assignment reports in Google Sheets

Built with Go, PostgreSQL and the Telegram and Google Sheets APIs. Deployed on Google Cloud Run.

## Google Sheets

Approvals add a name and date; returns fill the completion date. Later assignments keep the earlier history. The service-year tab and territory rows must be set up first.

![Sheets demo: approval writes a name and date, return updates the completion date, and reassignment adds a new column pair.](docs/sheets-preview.png)

## Development

See [setup instructions](api/README.md).

```sh
cd api
go build ./cmd/...
go test -race ./...
```

## Privacy and security

Some access checks still need fixes before new deployments with real user data. Keep residents' personal details out of notes, and real user data and credentials out of issues.

## License

[MIT](LICENSE)

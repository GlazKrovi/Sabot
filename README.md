# Sabot - Trading bot for Revolut X

Bot for automated trading using the official Revolut X API <https://developer.revolut.com/docs/x-api/revolut-x-crypto-exchange-rest-api>, according to golden cross strategy.

## Getting started

Run `go run ./setup` to help you generate the public key, private key, and API key required by Revolut X.

This setup simply follows the steps defined in <https://developer.revolut.com/docs/x-api/revolut-x-crypto-exchange-rest-api>

## Run

`go run ./cli ETH DOT`: checks that the keys are present (otherwise tells you to run `go run ./setup`), tests that the API key works through a dummy call to `/balances`, then starts the trading bot. This is the recommended command.

`go run . ETH DOT`: directly starts the trading bot without these checks.

`go run build && Sabot-private.exe`

### Logs

By default, logs are intentionally minimalistic ("production"): bot waiting state (Golden Cross "wait" suggestion, or initial market analysis), successful buy or sell order execution, and the balance status (USD + tracked cryptocurrencies) after each executed order.

Add the `-v` or `--verbose` flag (anywhere in the arguments, e.g. `go run ./cli ETH DOT -v`) to additionally display all detailed logs (iterations, current prices, signed API requests, etc.).

### Releases

Pushing a `vX.Y.Z` tag (`git tag v0.1.0 && git push origin v0.1.0`) triggers the GitHub Actions workflow which builds the `cli` binary (sabot) for Linux, Windows, and macOS on amd64 through [GoReleaser](https://goreleaser.com/) and publishes a GitHub release with the archives and checksums.

## Docs

### Why golang?

Because market prices change very quickly, a fast language capable of analyzing and executing "instantly" is required. Waiting should be something artificially added and not a limitation of the language or the tools!

### History and tracking

pnl_history.json allows you to see the total profits and losses realized by the application since its first launch on the user's machine. It also keeps a history of its transactions (quantity, USD amount, fees) and the fees it had to pay. This file is updated after each executed trade, based on the actual order state retrieved through `GET /orders/{id}`.

### Keys

private.pem => private key (not used directly in the code)

public.pem => public key (not used directly in the code, but by the Revolut X website)

API key => provided by Revolut after adding your public key to your profile; this is the key used in the headers, in the code

### ./setup process

> Details <https://developer.revolut.com/docs/x-api/revolut-x-crypto-exchange-rest-api>

`openssl genpkey -algorithm ed25519 -out ./private.pem`

`openssl pkey -in private.pem -pubout -out ./public.pem`

Add the **public** key in the Revolut X web app → Profile to complete the setup. An API key is provided to you; put it in an `api_rev_x.pem` file in this project.


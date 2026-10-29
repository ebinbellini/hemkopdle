# Hemköpdle

Guess the price of products [hemkop.se](https://www.hemkop.se).

## Requirements

- Go 1.27 or newer

## Configure

Copy .env.example to .env and edit the values therein.

## 3. Run

```bash
go run hemkopdle.go
```

Then open <http://localhost:2759>.

## Game modes

- Dagens vara: one product per day.
- Jag har tur: a random product with six guesses.
- Kund mot kund: compete in a lobby of 2–5 players

## Scheduling and politeness

<https://www.hemkop.se/robots.txt> asks for:

```
Crawl-delay: 10
Visit-time: 0400-0845      # UTC
```

The cron runs on `0 6 * * 1`, which is Monday morning local time.


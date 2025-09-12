# Joe Radio NL tracker

![Joe Radio NL](.static/logo.png)

Every morning when I take a shower, I put [Joe Radio NL](https://joe.nl) on, a
radio station dedicated to playing 70's, 80's and 90's hits. During one of these
showers I had a [thought](https://www.reddit.com/r/Showerthoughts/): their database
is limited by definition since no more 70's, 80's or 90's music is being made. So
I was wondering: how many songs would they even have in their database...?

I made a tracker that listens to their [live stream](https://joe.nl/luister),
looks up the song on spotify and checks if the song is already present in my
playlist. If not, add it and sooner or later we'll have all their songs mapped.
At least that is the idea!

## Running it

Make sure you have a developer account with Spotify, create a public or private
playlist, replace the playlist ID in `cmd/tracker/main.go` and put you Spotify
client ID and secret in `.env` or export in your environment. Then start the tracker

```bash
go run ./cmd/tracker
```

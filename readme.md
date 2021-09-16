# Poast

Post to twitter from slack

## Usage

1. Build: `go build .`
2. Copy config.json.example to config.json and fill out the details
  - Twitter credentials: make an app at https://developer.twitter.com/en/apps
  - Slack credentials:
    - Make a slack app at https://api.slack.com/apps
    - Enable socket mode and event subscriptions for these scopes:
      - app_mention
      - message.channels
      - message.groups
      - message.im
      - message.mpim
    - Under `Oauth & Permissions`, enable these scopes
      - app_mentions:read
      - channels:history
      - chat:write
      - groups:history
      - im:history
      - im:read
      - im:write
      - mpim:history
    - Install app
    - The app token is under `Basic Information` -> `App-Level Tokens` -> click the token name
    - The bot user token is under `Oauth & Permissions`
3. Run: `./poast`
4. Systemd: copy poast.service to /etc/systemd/system, make sure the paths there are correct, then `sudo systemd daemon-reload && sudo
systemctl start poast.service`

## Example

https://twitter.com/grin_io/status/1438611453121024005

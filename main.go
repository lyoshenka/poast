package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

var (
	mySlackUsername     string
	mySlackUserID       string
	myTwitterScreenName string
)

func main() {
	conf, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %s", err.Error())
		os.Exit(1)
	}

	slackApi := slack.New(
		conf.SlackBotToken,
		//slack.OptionDebug(true),
		//slack.OptionLog(log.New(os.Stdout, "api: ", log.Lshortfile|log.LstdFlags)),
		slack.OptionAppLevelToken(conf.SlackAppToken),
	)

	slackClient := socketmode.New(
		slackApi,
		//socketmode.OptionDebug(true),
		//socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	twitterConf := oauth1.NewConfig(conf.TwitterConsumerKey, conf.TwitterConsumerSecret) // consumer key / secret
	twitterToken := oauth1.NewToken(conf.TwitterAccessToken, conf.TwitterAccessSecret)   // access token / secret
	httpClient := twitterConf.Client(oauth1.NoContext, twitterToken)
	twitterClient := twitter.NewClient(httpClient)

	whoami(slackApi, twitterClient)

	go func() {
		for evt := range slackClient.Events {
			switch evt.Type {
			case socketmode.EventTypeConnecting:
				fmt.Println("Connecting to Slack with Socket Mode...")
			case socketmode.EventTypeConnectionError:
				fmt.Println("Connection failed. Retrying later...")
			case socketmode.EventTypeConnected:
				fmt.Println("Connected to Slack with Socket Mode.")
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					fmt.Printf("Ignored %+v\n", evt)

					continue
				}

				//fmt.Printf("Event received: %+v\n", eventsAPIEvent)

				slackClient.Ack(*evt.Request)

				switch eventsAPIEvent.Type {
				case slackevents.CallbackEvent:
					innerEvent := eventsAPIEvent.InnerEvent
					switch ev := innerEvent.Data.(type) {
					case *slackevents.AppMentionEvent:
						if ev.User == mySlackUserID {
							break // ignore bot's own messages
						}

						fmt.Printf("mentioned: %s\n", ev.Text)

						prefix := fmt.Sprintf(`<@%s> `, mySlackUserID)
						if !strings.HasPrefix(ev.Text, prefix) {
							slackApi.SendMessage(ev.Channel, slack.MsgOptionText(
								"Hi 👋. If you want me to tweet something, start your message with `@Poast `.\n"+
									"Feel free to make me tweet absolutely anything at all. Seriously, go wild. I won't mind.\n"+
									"You can even DM me if you want full privacy.", true))
							break
						}

						reply, err := tweet(twitterClient, ev.Text[len(prefix):])
						if err != nil {
							slackApi.SendMessage(ev.Channel, slack.MsgOptionText("ERROR: "+err.Error(), true))
							fmt.Printf("tweet err: %s\n", err.Error())
						} else {
							//log.Printf("https://twitter.com/%s/status/%s\n", meTwitter.ScreenName, tweet.IDStr)
							slackApi.SendMessage(ev.Channel, slack.MsgOptionText(reply, true))
						}

					case *slackevents.MessageEvent:
						if ev.ChannelType != "im" || ev.User == mySlackUserID {
							break // ignore bot's own messages
						}

						fmt.Printf("DMed: %s\n", ev.Text)

						prefix := fmt.Sprintf(`<@%s> `, mySlackUserID)
						text := ev.Text
						if strings.HasPrefix(text, prefix) {
							text = text[len(prefix):]
						}

						reply, err := tweet(twitterClient, text)
						if err != nil {
							slackApi.SendMessage(ev.Channel, slack.MsgOptionText("ERROR: "+err.Error(), true))
							fmt.Printf("tweet err: %s\n", err.Error())
						} else {
							//log.Printf("https://twitter.com/%s/status/%s\n", meTwitter.ScreenName, tweet.IDStr)
							slackApi.SendMessage(ev.Channel, slack.MsgOptionText(reply, true))
						}
					}
				default:
					slackClient.Debugf("unsupported Events API event received: %s", eventsAPIEvent.Type)
				}
			case socketmode.EventTypeHello,
				socketmode.EventTypeInteractive,
				socketmode.EventTypeSlashCommand:
				continue // ignored
			default:
				fmt.Fprintf(os.Stderr, "Unexpected event type received: %s\n", evt.Type)
			}
		}
	}()

	slackClient.Run()
}

func whoami(s *slack.Client, t *twitter.Client) {
	meSlack, err := s.AuthTest()
	if err != nil {
		fmt.Fprintf(os.Stderr, "slack auth test err: %s", err.Error())
		os.Exit(1)
	}
	mySlackUsername = meSlack.User
	mySlackUserID = meSlack.UserID
	fmt.Printf("I am %s on slack\n", mySlackUsername)

	meTwitter, _, err := t.Accounts.VerifyCredentials(&twitter.AccountVerifyParams{
		SkipStatus:      twitter.Bool(true),
		IncludeEmail:    twitter.Bool(false),
		IncludeEntities: twitter.Bool(false),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "twitter verify credentials err: %s", err.Error())
		os.Exit(1)
	}
	myTwitterScreenName = meTwitter.ScreenName
	fmt.Printf("I am %s on twitter\n", myTwitterScreenName)
}

func tweet(client *twitter.Client, message string) (string, error) {
	text := message + ` /c` // /c = communal

	tweet, _, err := client.Statuses.Update(text, nil)
	if err != nil {
		if apiError, ok := err.(twitter.APIError); ok && len(apiError.Errors) > 0 {
			return "", errors.New(apiError.Errors[0].Message)
		} else {
			return "", err
		}
	}

	//log.Printf("https://twitter.com/%s/status/%s\n", meTwitter.ScreenName, tweet.IDStr)
	return fmt.Sprintf("poasted! https://twitter.com/%s/status/%s", myTwitterScreenName, tweet.IDStr), nil
}

type config struct {
	SlackAppToken         string `json:"slack_app_token"`
	SlackBotToken         string `json:"slack_bot_token"`
	TwitterConsumerKey    string `json:"twitter_consumer_key"`
	TwitterConsumerSecret string `json:"twitter_consumer_secret"`
	TwitterAccessToken    string `json:"twitter_access_token"`
	TwitterAccessSecret   string `json:"twitter_access_secret"`
}

func loadConfig() (*config, error) {
	conf, err := os.ReadFile("config.json")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var c config

	err = json.Unmarshal(conf, &c)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if c.SlackAppToken == "" {
		return nil, errors.New("slack_app_token must be set.\n")
	}
	if !strings.HasPrefix(c.SlackAppToken, "xapp-") {
		return nil, errors.New("slack_app_token must have the prefix \"xapp-\".")
	}

	if c.SlackBotToken == "" {
		return nil, errors.New("slack_bot_token must be set.\n")
	}
	if !strings.HasPrefix(c.SlackBotToken, "xoxb-") {
		return nil, errors.New("slack_bot_token must have the prefix \"xoxb-\".")
	}

	return &c, nil
}

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

func main() {
	conf, err := loadConfig()
	if err != nil {
		panic(err)
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

	meSlack, err := slackApi.AuthTest()
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth test err: %s", err.Error())
		os.Exit(1)
	}
	fmt.Printf("I am %s on slack\n", meSlack.User)

	twitterConf := oauth1.NewConfig(conf.TwitterConsumerKey, conf.TwitterConsumerSecret) // consumer key / secret
	twitterToken := oauth1.NewToken(conf.TwitterAccessToken, conf.TwitterAccessSecret)   // access token / secret
	httpClient := twitterConf.Client(oauth1.NoContext, twitterToken)
	twitterClient := twitter.NewClient(httpClient)

	meTwitter, _, err := twitterClient.Accounts.VerifyCredentials(&twitter.AccountVerifyParams{
		SkipStatus:      twitter.Bool(true),
		IncludeEmail:    twitter.Bool(false),
		IncludeEntities: twitter.Bool(false),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "twitter verify credentials err: %s", err.Error())
		os.Exit(1)
	}
	fmt.Printf("I am %s on twitter\n", meTwitter.ScreenName)

	go func() {
		for evt := range slackClient.Events {
			switch evt.Type {
			case socketmode.EventTypeConnecting:
				fmt.Println("Connecting to Slack with Socket Mode...")
			case socketmode.EventTypeConnectionError:
				fmt.Println("Connection failed. Retrying later...")
			case socketmode.EventTypeConnected:
				fmt.Println("Connected to Slack with Socket Mode.")
			case socketmode.EventTypeHello:
				fmt.Println("Got hello message.")
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
						if ev.User == meSlack.UserID {
							break // ignore bot's own messages
						}

						fmt.Printf("mentioned: %s\n", ev.Text)
						text := ev.Text
						prefix := fmt.Sprintf(`<@%s> `, meSlack.UserID)
						if !strings.HasPrefix(text, prefix) {
							slackApi.SendMessage(ev.Channel, slack.MsgOptionText("Hi 👋. If you want me to tweet something, start your message with `@Poast `.", true))
							break
						}

						text = ev.Text[len(prefix):] + ` /c` // /c = communal

						tweet, _, err := twitterClient.Statuses.Update(text, nil)
						if err != nil {
							if apiError, ok := err.(twitter.APIError); ok && len(apiError.Errors) > 0 {
								slackApi.SendMessage(ev.Channel, slack.MsgOptionText(fmt.Sprintf("ERROR: %s", apiError.Errors[0].Message), true))
							} else {
								slackApi.SendMessage(ev.Channel, slack.MsgOptionText(fmt.Sprintf("ERROR: %s", err.Error()), true))
							}
							fmt.Printf("tweet err: %s\n", err.Error())
							break
						}

						//log.Printf("https://twitter.com/%s/status/%s\n", meTwitter.ScreenName, tweet.IDStr)
						slackApi.SendMessage(ev.Channel, slack.MsgOptionText(fmt.Sprintf("poasted! https://twitter.com/%s/status/%s", meTwitter.ScreenName, tweet.IDStr), true))

					case *slackevents.MemberJoinedChannelEvent:
						//fmt.Printf("user %q joined to channel %q", ev.User, ev.Channel)
					case *slackevents.MessageEvent:
						//fmt.Printf("%s said %s\n", ev.User, ev.Text)
					}
				default:
					slackClient.Debugf("unsupported Events API event received: %s", eventsAPIEvent.Type)
				}
			case socketmode.EventTypeInteractive:
				continue // ignored

				callback, ok := evt.Data.(slack.InteractionCallback)
				if !ok {
					fmt.Printf("Ignored %+v\n", evt)
					continue
				}

				fmt.Printf("Interaction received: %+v\n", callback)

				var payload interface{}

				switch callback.Type {
				case slack.InteractionTypeBlockActions:
					// See https://api.slack.com/apis/connections/socket-implement#button

					slackClient.Debugf("button clicked!")
				case slack.InteractionTypeShortcut:
				case slack.InteractionTypeViewSubmission:
					// See https://api.slack.com/apis/connections/socket-implement#modal
				case slack.InteractionTypeDialogSubmission:
				default:

				}

				slackClient.Ack(*evt.Request, payload)
			case socketmode.EventTypeSlashCommand:
				continue // ignored

				cmd, ok := evt.Data.(slack.SlashCommand)
				if !ok {
					fmt.Printf("Ignored %+v\n", evt)
					continue
				}

				slackClient.Debugf("Slash command received: %+v", cmd)

				payload := map[string]interface{}{
					"blocks": []slack.Block{
						slack.NewSectionBlock(
							&slack.TextBlockObject{
								Type: slack.MarkdownType,
								Text: "foo",
							},
							nil,
							slack.NewAccessory(
								slack.NewButtonBlockElement(
									"",
									"somevalue",
									&slack.TextBlockObject{
										Type: slack.PlainTextType,
										Text: "bar",
									},
								),
							),
						),
					}}

				slackClient.Ack(*evt.Request, payload)
			default:
				fmt.Fprintf(os.Stderr, "Unexpected event type received: %s\n", evt.Type)
			}
		}
	}()

	slackClient.Run()
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

package notifier

import (
	"fmt"
	"log"

	"github.com/slack-go/slack"
)

type SlackNotifier struct {
	client  *slack.Client
	channel string
}

func NewSlackNotifier(botToken, channel string) *SlackNotifier {
	return &SlackNotifier{
		client:  slack.New(botToken),
		channel: channel,
	}
}

func (s *SlackNotifier) Notify(message string) error {
	return s.NotifyWithButtons(message, "")
}

func (s *SlackNotifier) NotifyWithButtons(message, alertID string) error {
	if alertID == "" {
		alertID = fmt.Sprintf("alert_%d", len(message))
	}

	acknowledgeBtn := slack.NewButtonBlockElement("acknowledge", alertID, slack.NewTextBlockObject("plain_text", "✅ Acknowledge", false, false))
	acknowledgeBtn.Style = slack.StylePrimary

	dismissBtn := slack.NewButtonBlockElement("dismiss", alertID, slack.NewTextBlockObject("plain_text", "✖️ Dismiss", false, false))
	dismissBtn.Style = slack.StyleDanger

	actionBlock := slack.NewActionBlock("alert_actions", acknowledgeBtn, dismissBtn)

	headerSection := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("🚨 *Alert*\n%s", message), false, false),
		nil, nil,
	)

	blocks := []slack.Block{
		headerSection,
		actionBlock,
	}

	_, _, err := s.client.PostMessage(s.channel,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(message, false),
	)

	if err != nil {
		log.Printf("Failed to send Slack message: %v", err)
		return err
	}

	return nil
}

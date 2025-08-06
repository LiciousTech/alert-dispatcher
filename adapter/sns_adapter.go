package sns_adapter

import (
	"encoding/json"
	"fmt"
)

type SNSMessage struct {
	AlarmName string `json:"AlarmName"`
	NewState  string `json:"NewStateValue"`
	Reason    string `json:"NewStateReason"`
}

func AdaptSQSMessage(body string) (string, error) {
	var envelope struct {
		Message string `json:"Message"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return "", err
	}

	var snsMsg SNSMessage
	if err := json.Unmarshal([]byte(envelope.Message), &snsMsg); err != nil {
		return "", err
	}

	return fmt.Sprintf("*%s* is now *%s*:\n%s", snsMsg.AlarmName, snsMsg.NewState, snsMsg.Reason), nil
}

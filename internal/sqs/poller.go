package sqs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type Poller struct {
	Client   *sqs.Client
	QueueURL string
}

func NewPoller(queueURL string) (*Poller, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, err
	}
	client := sqs.NewFromConfig(cfg)
	return &Poller{
		Client:   client,
		QueueURL: queueURL,
	}, nil
}

func (p *Poller) Poll(handler func(string) error) {
	for {
		out, err := p.Client.ReceiveMessage(context.TODO(), &sqs.ReceiveMessageInput{
			QueueUrl:            &p.QueueURL,
			MaxNumberOfMessages: 5,
			WaitTimeSeconds:     10,
		})
		if err != nil {
			log.Printf("Receive error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, msg := range out.Messages {
			fmt.Println("Processing message:", *msg.Body)

			if err := handler(*msg.Body); err == nil {
				// Delete message on success
				_, err := p.Client.DeleteMessage(context.TODO(), &sqs.DeleteMessageInput{
					QueueUrl:      &p.QueueURL,
					ReceiptHandle: msg.ReceiptHandle,
				})
				if err != nil {
					log.Printf("Delete error: %v", err)
				}
			} else {
				log.Printf("Handler error: %v", err)
			}
		}
	}
}

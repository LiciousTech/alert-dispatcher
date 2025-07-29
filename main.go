package main

import (
	"log"
	"sync"
	"time"

	"alert-dispatcher/internal/adapter"
	"alert-dispatcher/internal/config"
	"alert-dispatcher/internal/server"
	"alert-dispatcher/internal/sqs"
	"alert-dispatcher/notifier"
)

func main() {
	cfg := config.LoadConfig()

	poller, err := sqs.NewPoller(cfg.SQSQueueURL)
	if err != nil {
		log.Fatalf("Failed to create poller: %v", err)
	}


	handler := func(body string) error {
		alertMsg, err := adapter.AdaptSQSMessageWithRouting(body, cfg.SlackChannels, cfg.AlarmChannels)
		if err != nil {
			return err
		}
		
		// Create notifier for specific channel
		channelNotifier := notifier.NewSlackNotifier(cfg.SlackBotToken, alertMsg.Channel)
		log.Printf("Sending %s alert to %s", alertMsg.Priority, alertMsg.Channel)
		
		return channelNotifier.Notify(alertMsg.Message)
	}

	srv := server.NewServer(cfg.SlackSigningSecret, cfg.ServerPort, cfg)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		log.Println("Starting HTTP server...")
		if err := srv.Start(); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		log.Println("Starting SQS polling...")
		for {
			poller.Poll(handler)
			time.Sleep(time.Duration(cfg.PollIntervalSec) * time.Second)
		}
	}()

	wg.Wait()
}

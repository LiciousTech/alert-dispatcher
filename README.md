# Alert Dispatcher

A Go-based microservice that processes AWS CloudWatch alarms from SNS/SQS and Grafana webhook alerts, sending intelligent, interactive notifications to Slack channels based on priority levels.

![Architecture Diagram](./Architecture.png)

## Features

- **SNS/SQS Message Processing**: Continuously polls AWS SQS for SNS-wrapped CloudWatch alarm notifications
- **Priority-Based Routing**: Automatically routes alerts to different Slack channels (P0, P1, P2) based on alarm characteristics
- **Interactive Slack Messages**: Rich formatted messages with acknowledge/dismiss buttons
- **Multi-Service Support**: Works with all AWS services (EC2, RDS, Lambda, ELB, ECR, etc.)
- **Concurrent Processing**: Runs SQS polling and HTTP server concurrently
- **Grafana Support**: Supports grafana out of the box.
- **Security**: Request signature verification for Slack interactions

## 📋 Flow Diagram


```
┌─────────────────┐    ┌──────────────────┐    ┌──────────────────┐
│   CloudWatch    │───▶│       SNS        │───▶│       SQS        │
│     Alarms      │    │      Topic       │    │      Queue       │
└─────────────────┘    └──────────────────┘    └──────────────────┘
                                                         │
┌─────────────────┐                                      │
│    Grafana      │────────────────────────────────────-┐│
│    Alerts       │         HTTP Webhook                ││
└─────────────────┘         POST /webhooks/grafana      ││
                                                        ││
                                              ┌─────────▼▼─────────-┐
                                              │  Alert Dispatcher   │◀────────────────────────-┐
                                              │ ┌─────────────────┐ │                          │
                                              │ │  SQS Poller     │ │                          │
                                              │ │  Webhook Server │ │                          │
                                              │ │  Message Adapter│ │                          │
                                              │ │  Priority Router│ │                          │
                                              │ │  Slack Notifier │ │                          │
                                              │ └─────────────────┘ │                          │
                                              └─────────┬───────────┘                          │
                                                        │                                      │
                                              ┌─────────▼───────────┐                          │
                                              │   Slack Channels    │                          │
                                              │ • #p0-channel       │                          │
                                              │ • #p1-channel       │                          │
                                              │ • #p2-channel       │                          │
                                              └─────────┬───────────┘                          │
                                                        │                                      │ 
                                                        └────────────Slack Interactivity──────-┘
                                                                     (ACK / Dismiss buttons)

```

### Message Flow

1. **CloudWatch Alarms** trigger when thresholds are breached
2. **SNS Topic** receives the alarm notification
3. **SQS Queue** is subscribed to the SNS topic and receives the message
4. **Alert Dispatcher** polls SQS, processes the SNS-wrapped CloudWatch alarm and also listen to webhook for grafana alerts
5. **Slack Channels** receive formatted, interactive notifications based on priority

## 🏗️ Components

- **SQS Poller**: Continuously polls AWS SQS for SNS messages
- **Message Adapter**: Parses SNS-wrapped CloudWatch alarm JSON and formats for Slack
- **Priority Router**: Determines alert priority and target channel
- **Slack Notifier**: Sends interactive messages to Slack
- **HTTP Server**: Handles Slack interactive component callbacks
- **Webhook Server**: For grafana alerts

## 📦 Installation

### Prerequisites

- Go 1.21+
- AWS account with SQS access
- Slack Bot with appropriate permissions
- Kubernetes cluster (for production deployment)

### Local Development

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd alert-dispatcher
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Set environment variables**
   ```bash
   export SQS_QUEUE_URL="https://sqs.<region>.amazonaws.com/123456789012/your-queue"
   export SLACK_BOT_TOKEN="xoxb-your-bot-token"
   export SLACK_SIGNING_SECRET="your-signing-secret"
   export SLACK_CHANNEL_P0="#p0-channel"
   export SLACK_CHANNEL_P1="#p1-channel"
   export SLACK_CHANNEL_P2="#p2-channel"
   export SLACK_CHANNEL_DEFAULT="#alerts"
   export SERVER_PORT="8088"
   export POLL_INTERVAL_SEC="10"
   ```

4. **Run the application**
   ```bash
   go run main.go
   ```

## 🔧 Configuration

### Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `SQS_QUEUE_URL` | AWS SQS queue URL | ✅ | - |
| `SLACK_BOT_TOKEN` | Slack bot OAuth token | ✅ | - |
| `SLACK_SIGNING_SECRET` | Slack app signing secret | ✅ | - |
| `SERVER_PORT` | HTTP server port | ❌ | 8088 |
| `POLL_INTERVAL_SEC` | SQS polling interval | ❌ | 10 |
| `SLACK_CHANNEL_P0` | Critical alerts channel | ❌ | #p0-channel |
| `SLACK_CHANNEL_P1` | Important alerts channel | ❌ | #p1-channel |
| `SLACK_CHANNEL_P2` | Normal alerts channel | ❌ | #p2-channel |
| `SLACK_CHANNEL_DEFAULT` | Fallback channel | ❌ | #alerts |

### Priority Routing Logic

**P0 (Critical)** → `#p0-channel`:
- Alarms containing "prod" or "production" in name
- RDS and DynamoDB services

**P1 (Important)** → `#p1-channel`:
- ELB and Lambda services
- CPU/Memory related alarms

**P2 (Normal)** → `#p2-channel`:
- All other alarms (ECR, S3, etc.)

## 📱 Slack Setup

### 1. Create Slack App

1. Go to https://api.slack.com/apps
2. Create new app "From scratch"
3. Add Bot Token Scopes:
   - `chat:write`
   - `chat:write.public`
   - `channels:read`

### 2. Configure Interactive Components

1. Go to "Interactivity & Shortcuts"
2. Enable Interactivity
3. Set Request URL: `https://your-domain.com/slack/events`

### 3. Install and Configure

1. Install app to workspace
2. Copy Bot User OAuth Token and Signing Secret
3. Invite bot to all alert channels:
   ```
   /invite @YourBotName
   ```

## 🐳 Docker Deployment

### Build Image

```bash
docker build -t alert-dispatcher:latest .
```

### Run Container

```bash
docker run -d \
  --name alert-dispatcher \
  -p 8088:8088 \
  -e SQS_QUEUE_URL="your-sqs-url" \
  -e SLACK_BOT_TOKEN="your-bot-token" \
  -e SLACK_SIGNING_SECRET="your-signing-secret" \
  alert-dispatcher:latest
```

## Kubernetes Deployment

### Quick Deploy

```bash
# 1. Update configuration in k8s/ files
# 2. Create secrets
kubectl create secret generic alert-dispatcher-secrets \
  --namespace=alert-dispatcher \
  --from-literal=SQS_QUEUE_URL="your-sqs-url" \
  --from-literal=SLACK_BOT_TOKEN="your-bot-token" \
  --from-literal=SLACK_SIGNING_SECRET="your-signing-secret"

# 3. Deploy
kubectl apply -f k8s/

# 4. Check status
kubectl get pods -n alert-dispatcher
```

See [k8s/README.md](k8s/README.md) for detailed deployment instructions.

## AWS Permissions

The service requires minimal SQS permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "sqs:ReceiveMessage",
        "sqs:DeleteMessage",
        "sqs:GetQueueAttributes"
      ],
      "Resource": "arn:aws:sqs:*:YOUR_ACCOUNT_ID:*"
    }
  ]
}
```

## Message Format

Alerts appear in Slack with rich formatting:

```
🚨 [ALARM] CloudWatch Alarm: ECR-PULL-COUNT-TESTING
• From: OK → To: ALARM
• Metric: AWS/ECR/RepositoryPullCount
• Threshold: GreaterThanThreshold 1.0
• Period: 60s over 1 evaluations
• Dimensions:
   → RepositoryName: test-repo
• Region: Asia Pacific (Mumbai)
• Reason: Threshold Crossed: 1 out of the last 1 datapoints...
• Time: 2025-07-24 13:00:00 UTC

[✅ Acknowledge] [❌ Dismiss]
```

## Testing

### Send Test SQS Message

```bash
aws sqs send-message \
  --queue-url "your-sqs-queue-url" \
  --message-body '{
    "Type": "Notification",
    "MessageId": "test-message-id",
    "TopicArn": "arn:aws:sns:us-east-1:123456789012:test-topic",
    "Subject": "ALARM: \"TEST-ALARM\" in US East (N. Virginia)",
    "Message": "{\"AlarmName\":\"TEST-ALARM\",\"NewStateValue\":\"ALARM\",\"OldStateValue\":\"OK\",\"NewStateReason\":\"Test alarm for development\",\"StateChangeTime\":\"2025-07-24T13:00:00.000+0000\",\"Region\":\"US East (N. Virginia)\",\"Trigger\":{\"MetricName\":\"CPUUtilization\",\"Namespace\":\"AWS/EC2\",\"ComparisonOperator\":\"GreaterThanThreshold\",\"Threshold\":80.0,\"Period\":300,\"EvaluationPeriods\":2,\"Dimensions\":[{\"name\":\"InstanceId\",\"value\":\"i-1234567890abcdef0\"}]}}",
    "Timestamp": "2025-07-24T13:00:00.000Z"
  }'
```

### Health Check

```bash
curl http://localhost:8088/health
# Should return: OK
```

## 📝 Logging

The application provides structured logging for:
- SQS message processing
- Priority routing decisions
- Slack message delivery
- Interactive button responses
- Error handling

Example logs:
```
2025/07/24 13:33:30 Starting HTTP server...
2025/07/24 13:33:30 Starting SQS polling...
2025/07/24 13:33:45 Processing message: {...}
2025/07/24 13:33:45 Sending P1 alert to #p1-channel
2025/07/24 13:34:12 Alert alert_123 acknowledged by john.doe
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## 📄 License

[Add your license here]

## Support

For issues and questions:
1. Check the logs: `kubectl logs -f deployment/alert-dispatcher -n alert-dispatcher`
2. Verify Slack app configuration
3. Ensure AWS permissions are correct
4. Test health endpoint: `/health`

---

**Built for reliable infrastructure monitoring**
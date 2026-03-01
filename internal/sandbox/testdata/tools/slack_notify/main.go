// slack_notify WASM tool — Post messages to Slack via incoming webhook.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o slack_notify.wasm .
package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/bitop-dev/agent-core/pkg/hostcall"
)

type request struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Config    map[string]any  `json:"config"`
}

type notifyArgs struct {
	WebhookURL string `json:"webhook_url"`
	Text       string `json:"text"`
	Channel    string `json:"channel"`
	Username   string `json:"username"`
}

type result struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

func main() {
	input, _ := io.ReadAll(os.Stdin)

	var req request
	if err := json.Unmarshal(input, &req); err != nil {
		writeResult(result{Content: "invalid input: " + err.Error(), IsError: true})
		return
	}

	var args notifyArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		writeResult(result{Content: "invalid arguments: " + err.Error(), IsError: true})
		return
	}

	if args.WebhookURL == "" {
		writeResult(result{Content: "webhook_url is required", IsError: true})
		return
	}
	if args.Text == "" {
		writeResult(result{Content: "text is required", IsError: true})
		return
	}

	// Build Slack payload
	payload := map[string]string{"text": args.Text}
	if args.Channel != "" {
		payload["channel"] = args.Channel
	}
	if args.Username != "" {
		payload["username"] = args.Username
	}

	body, _ := json.Marshal(payload)

	resp, err := hostcall.HTTPPost(args.WebhookURL, body)
	if err != nil {
		writeResult(result{Content: "slack post failed: " + err.Error(), IsError: true})
		return
	}

	writeResult(result{Content: "sent: " + string(resp)})
}

func writeResult(r result) {
	json.NewEncoder(os.Stdout).Encode(r)
}

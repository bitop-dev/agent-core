package main
import ("encoding/json";"io";"os";"github.com/bitop-dev/agent-core/pkg/hostcall")
type request struct{Name string`json:"name"`;Arguments json.RawMessage`json:"arguments"`}
type result struct{Content string`json:"content"`;IsError bool`json:"is_error"`}
func main(){
	input,_:=io.ReadAll(os.Stdin);var req request;json.Unmarshal(input,&req)
	var a struct{WebhookURL,Content,Username,EmbedTitle,EmbedDescription string;EmbedColor int}
	json.Unmarshal(req.Arguments,&a)
	if a.WebhookURL==""{a.WebhookURL=os.Getenv("DISCORD_WEBHOOK_URL")}
	if a.WebhookURL==""{wr(result{"webhook_url or DISCORD_WEBHOOK_URL required",true});return}
	payload:=map[string]any{"content":a.Content}
	if a.Username!=""{payload["username"]=a.Username}
	if a.EmbedTitle!=""{payload["embeds"]=[]any{map[string]any{"title":a.EmbedTitle,"description":a.EmbedDescription,"color":a.EmbedColor}}}
	body,_:=json.Marshal(payload)
	resp,err:=hostcall.HTTPPost(a.WebhookURL,body)
	if err!=nil{wr(result{"discord post failed: "+err.Error(),true});return}
	wr(result{"sent: "+string(resp),false})
}
func wr(r result){json.NewEncoder(os.Stdout).Encode(r)}

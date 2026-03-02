package main
import ("encoding/json";"io";"os";"github.com/bitop-dev/agent-core/pkg/hostcall")
type request struct{Name string`json:"name"`;Arguments json.RawMessage`json:"arguments"`}
type result struct{Content string`json:"content"`;IsError bool`json:"is_error"`}
func main(){
	input,_:=io.ReadAll(os.Stdin);var req request;json.Unmarshal(input,&req)
	key:=os.Getenv("RESEND_API_KEY");if key==""{wr(result{"RESEND_API_KEY required",true});return}
	var a struct{To,Subject,Body,From string}
	json.Unmarshal(req.Arguments,&a)
	if a.From==""{a.From="onboarding@resend.dev"}
	body,_:=json.Marshal(map[string]any{"from":a.From,"to":[]string{a.To},"subject":a.Subject,"html":"<pre>"+a.Body+"</pre>"})
	h:=map[string]string{"Authorization":"Bearer "+key,"Content-Type":"application/json"}
	resp,err:=hostcall.HTTPRequestWithHeaders("POST","https://api.resend.com/emails",h,body)
	if err!=nil{wr(result{"send failed: "+err.Error(),true});return}
	wr(result{string(resp),false})
}
func wr(r result){json.NewEncoder(os.Stdout).Encode(r)}

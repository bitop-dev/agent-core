package main
import ("encoding/json";"fmt";"io";"os";"github.com/bitop-dev/agent-core/pkg/hostcall")
type request struct{Name string`json:"name"`;Arguments json.RawMessage`json:"arguments"`}
type result struct{Content string`json:"content"`;IsError bool`json:"is_error"`}
func main(){
	input,_:=io.ReadAll(os.Stdin);var req request;json.Unmarshal(input,&req)
	token:=os.Getenv("GITHUB_TOKEN");if token==""{wr(result{"GITHUB_TOKEN required",true});return}
	h:=map[string]string{"Authorization":"Bearer "+token,"Accept":"application/vnd.github+json","X-GitHub-Api-Version":"2022-11-28"}
	switch req.Name{
	case "gha_list_runs":
		var a struct{Owner,Repo,Workflow,Status string;Limit int};json.Unmarshal(req.Arguments,&a)
		if a.Limit==0{a.Limit=10}
		u:=fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs?per_page=%d",a.Owner,a.Repo,a.Limit)
		if a.Status!=""{u+="&status="+a.Status}
		resp,err:=hostcall.HTTPRequestWithHeaders("GET",u,h,nil)
		if err!=nil{wr(result{err.Error(),true});return}
		wr(result{string(resp),false})
	case "gha_trigger_workflow":
		var a struct{Owner,Repo,Workflow,Ref,Inputs string};json.Unmarshal(req.Arguments,&a)
		if a.Ref==""{a.Ref="main"}
		payload:=map[string]any{"ref":a.Ref}
		body,_:=json.Marshal(payload)
		u:=fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/workflows/%s/dispatches",a.Owner,a.Repo,a.Workflow)
		_,err:=hostcall.HTTPRequestWithHeaders("POST",u,h,body)
		if err!=nil{wr(result{err.Error(),true});return}
		wr(result{"workflow dispatched",false})
	default:wr(result{"unknown: "+req.Name,true})
	}
}
func wr(r result){json.NewEncoder(os.Stdout).Encode(r)}

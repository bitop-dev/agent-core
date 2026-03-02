package main
import ("encoding/json";"fmt";"io";"os";"github.com/bitop-dev/agent-core/pkg/hostcall")
type request struct{Name string`json:"name"`;Arguments json.RawMessage`json:"arguments"`}
type result struct{Content string`json:"content"`;IsError bool`json:"is_error"`}
func main(){
	input,_:=io.ReadAll(os.Stdin);var req request;json.Unmarshal(input,&req)
	key:=os.Getenv("LINEAR_API_KEY");if key==""{wr(result{"LINEAR_API_KEY required",true});return}
	h:=map[string]string{"Authorization":key,"Content-Type":"application/json"}
	switch req.Name{
	case "linear_list_issues":
		var a struct{Team,State string;Limit int}
		json.Unmarshal(req.Arguments,&a);if a.Limit==0{a.Limit=20}
		q:=fmt.Sprintf(`{"query":"{ issues(first:%d){ nodes{ id identifier title state{name} priority } } }"}`,a.Limit)
		resp,err:=hostcall.HTTPRequestWithHeaders("POST","https://api.linear.app/graphql",h,[]byte(q))
		if err!=nil{wr(result{err.Error(),true});return}
		wr(result{string(resp),false})
	case "linear_create_issue":
		var a struct{Title,Description,Team string;Priority int}
		json.Unmarshal(req.Arguments,&a)
		q:=fmt.Sprintf(`{"query":"mutation{ issueCreate(input:{title:\"%s\",teamId:\"%s\",priority:%d}){ success issue{ id identifier url } } }"}`,a.Title,a.Team,a.Priority)
		resp,err:=hostcall.HTTPRequestWithHeaders("POST","https://api.linear.app/graphql",h,[]byte(q))
		if err!=nil{wr(result{err.Error(),true});return}
		wr(result{string(resp),false})
	default:wr(result{"unknown: "+req.Name,true})
	}
}
func wr(r result){json.NewEncoder(os.Stdout).Encode(r)}

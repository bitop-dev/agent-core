package main
import ("encoding/json";"io";"os";"github.com/bitop-dev/agent-core/pkg/hostcall")
type request struct{Name string`json:"name"`;Arguments json.RawMessage`json:"arguments"`}
type result struct{Content string`json:"content"`;IsError bool`json:"is_error"`}
func main(){
	input,_:=io.ReadAll(os.Stdin);var req request;json.Unmarshal(input,&req)
	key:=os.Getenv("NOTION_API_KEY");if key==""{wr(result{"NOTION_API_KEY required",true});return}
	h:=map[string]string{"Authorization":"Bearer "+key,"Notion-Version":"2022-06-28","Content-Type":"application/json"}
	switch req.Name{
	case "notion_search":
		var a struct{Query string};json.Unmarshal(req.Arguments,&a)
		body,_:=json.Marshal(map[string]any{"query":a.Query})
		resp,err:=hostcall.HTTPRequestWithHeaders("POST","https://api.notion.com/v1/search",h,body)
		if err!=nil{wr(result{err.Error(),true});return}
		wr(result{string(resp),false})
	case "notion_create_page":
		var a struct{ParentID,Title,Content string`json:"parent_id"`};json.Unmarshal(req.Arguments,&a)
		page:=map[string]any{"parent":map[string]string{"page_id":a.ParentID},"properties":map[string]any{"title":map[string]any{"title":[]any{map[string]any{"text":map[string]string{"content":a.Title}}}}}}
		body,_:=json.Marshal(page)
		resp,err:=hostcall.HTTPRequestWithHeaders("POST","https://api.notion.com/v1/pages",h,body)
		if err!=nil{wr(result{err.Error(),true});return}
		wr(result{string(resp),false})
	default:wr(result{"unknown: "+req.Name,true})
	}
}
func wr(r result){json.NewEncoder(os.Stdout).Encode(r)}

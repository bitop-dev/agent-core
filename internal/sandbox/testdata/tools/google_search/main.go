package main
import ("encoding/json";"fmt";"io";"net/url";"os";"github.com/bitop-dev/agent-core/pkg/hostcall")
type request struct{Name string`json:"name"`;Arguments json.RawMessage`json:"arguments"`}
type result struct{Content string`json:"content"`;IsError bool`json:"is_error"`}
func main(){
	input,_:=io.ReadAll(os.Stdin);var req request;json.Unmarshal(input,&req)
	key:=os.Getenv("GOOGLE_SEARCH_API_KEY");cx:=os.Getenv("GOOGLE_SEARCH_CX")
	if key==""||cx==""{wr(result{"GOOGLE_SEARCH_API_KEY and GOOGLE_SEARCH_CX required",true});return}
	var a struct{Query string;Num int}
	json.Unmarshal(req.Arguments,&a)
	if a.Num==0{a.Num=5}
	u:=fmt.Sprintf("https://www.googleapis.com/customsearch/v1?key=%s&cx=%s&q=%s&num=%d",key,cx,url.QueryEscape(a.Query),a.Num)
	resp,err:=hostcall.HTTPGet(u)
	if err!=nil{wr(result{"search failed: "+err.Error(),true});return}
	wr(result{string(resp),false})
}
func wr(r result){json.NewEncoder(os.Stdout).Encode(r)}

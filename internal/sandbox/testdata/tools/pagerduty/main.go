package main
import ("encoding/json";"fmt";"io";"os";"github.com/bitop-dev/agent-core/pkg/hostcall")
type request struct{Name string`json:"name"`;Arguments json.RawMessage`json:"arguments"`}
type result struct{Content string`json:"content"`;IsError bool`json:"is_error"`}
func main(){
	input,_:=io.ReadAll(os.Stdin);var req request;json.Unmarshal(input,&req)
	key:=os.Getenv("PAGERDUTY_API_KEY");if key==""{wr(result{"PAGERDUTY_API_KEY required",true});return}
	h:=map[string]string{"Authorization":"Token token="+key,"Content-Type":"application/json","Accept":"application/vnd.pagerduty+json;version=2"}
	switch req.Name{
	case "pd_list_incidents":
		var a struct{Status string;Limit int};json.Unmarshal(req.Arguments,&a)
		if a.Limit==0{a.Limit=20}
		u:=fmt.Sprintf("https://api.pagerduty.com/incidents?limit=%d",a.Limit)
		if a.Status!=""{u+="&statuses[]="+a.Status}
		resp,err:=hostcall.HTTPRequestWithHeaders("GET",u,h,nil)
		if err!=nil{wr(result{err.Error(),true});return}
		wr(result{string(resp),false})
	case "pd_create_incident":
		var a struct{Title,ServiceID,Urgency,Body string`json:"service_id"`};json.Unmarshal(req.Arguments,&a)
		if a.Urgency==""{a.Urgency="high"}
		inc:=map[string]any{"type":"incident","title":a.Title,"service":map[string]string{"id":a.ServiceID,"type":"service_reference"},"urgency":a.Urgency}
		body,_:=json.Marshal(map[string]any{"incident":inc})
		resp,err:=hostcall.HTTPRequestWithHeaders("POST","https://api.pagerduty.com/incidents",h,body)
		if err!=nil{wr(result{err.Error(),true});return}
		wr(result{string(resp),false})
	case "pd_resolve":
		var a struct{IncidentID string`json:"incident_id"`};json.Unmarshal(req.Arguments,&a)
		body,_:=json.Marshal(map[string]any{"incident":map[string]any{"type":"incident_reference","status":"resolved"}})
		resp,err:=hostcall.HTTPRequestWithHeaders("PUT","https://api.pagerduty.com/incidents/"+a.IncidentID,h,body)
		if err!=nil{wr(result{err.Error(),true});return}
		wr(result{string(resp),false})
	default:wr(result{"unknown: "+req.Name,true})
	}
}
func wr(r result){json.NewEncoder(os.Stdout).Encode(r)}

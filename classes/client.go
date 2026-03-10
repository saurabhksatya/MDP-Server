package classes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
)

type Client struct {
	ClientId string
	Conn     *websocket.Conn
}

type authClientResp struct {
	Success bool `json:"success"`
}

func AuthenticateClient(ClientId string, secret string) bool {
	fmt.Println("Authenticating Client")
	req, err := http.NewRequest("GET", "http://localhost:3000/api/ws/authClient/"+ClientId+"/"+secret, nil)
	if err != nil {
		fmt.Println("Req Error")
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authorization", "HelloWorld")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return false
	}

	defer res.Body.Close()
	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		return false
	}
	jsonResp := authDeviceResp{}
	err = json.Unmarshal(respBody, &jsonResp)
	if err != nil {
		fmt.Println(err)
		return false
	}
	fmt.Println(jsonResp.Success, jsonResp.ClientId)
	if jsonResp.Success != true {
		return false
	}
	return true
}

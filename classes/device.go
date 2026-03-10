package classes

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

type Device struct {
	DeviceID string
	ClientID string
	Conn     *websocket.Conn
}

type authDeviceResp struct {
	Success  bool   `json:"success"`
	ClientId string `json:"clientId"`
}

func AuthenticateDevice(deviceID string, secret string) (bool, string) {
	fmt.Println("Authenticating Device")

	err := godotenv.Load()

	if err != nil {
		log.Println("Cannot load .env file")
	}

	req, err := http.NewRequest("GET", os.Getenv("CLIENT_ADDRESS")+"/api/ws/authDevice/"+deviceID+"/"+secret, nil)
	if err != nil {
		fmt.Println("Req Error")
		return false, ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authorization", os.Getenv("CLIENT_SECRET"))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return false, ""
	}

	defer res.Body.Close()
	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		return false, ""
	}
	jsonResp := authDeviceResp{}
	err = json.Unmarshal(respBody, &jsonResp)
	if err != nil {
		fmt.Println(err)
		return false, ""
	}
	fmt.Println(jsonResp.Success, jsonResp.ClientId)
	if jsonResp.Success != true {
		return false, ""
	}
	return true, jsonResp.ClientId
}

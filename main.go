package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	WEBSOCKET_CONN_READ_TIMEOUT = 60 * 1
	WEBSOCKET_CONN_WRITE_TIMEOUT = 60 * 1

	WSSVR_MAX_READ_BUFFER_SIZE = 30 * 1024
	WSSVR_MAX_WRITE_BUFFER_SIZE = 30 * 1024
)


var upgrader = websocket.Upgrader{
	ReadBufferSize:  WSSVR_MAX_READ_BUFFER_SIZE,
	WriteBufferSize: WSSVR_MAX_WRITE_BUFFER_SIZE,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func dialBackendWsNode(r *http.Request) (*websocket.Conn, error) {
	u := url.URL{Scheme: "ws", Host: ":8081", Path: "/"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println("dialBackendWsNode failed, err: ", err)
		return nil, err
	}
	return c, nil
}

func ProxyPass(from, to *websocket.Conn, complete, done, otherDone chan bool) {
	for {
		select {
		case <- otherDone:
			complete <- true
			return
		default:
			from.SetReadLimit(WSSVR_MAX_READ_BUFFER_SIZE)
			from.SetReadDeadline(time.Now().Add(time.Second * WEBSOCKET_CONN_READ_TIMEOUT))
			from.SetPongHandler(func(string)error {
				from.SetReadDeadline(time.Now().Add(time.Second * WEBSOCKET_CONN_READ_TIMEOUT))
				return nil
			})

			_, message, err := from.ReadMessage()
			if err != nil {
				complete <- true
				done <- true
				return
			}


			to.SetWriteDeadline(time.Now().Add(time.Second * WEBSOCKET_CONN_WRITE_TIMEOUT))
			err = to.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				complete <- true
				done <- true
				return
			}
		}
	}
}

func proxyPass(from, to *websocket.Conn) {
	ch1 := make(chan bool, 1)
	ch2 := make(chan bool, 1)

	complete := make(chan bool, 2)

	go ProxyPass(from, to, complete, ch1, ch2)
	go ProxyPass(to, from, complete, ch2, ch1)

	<-complete
	<-complete

	fmt.Println("proxy complete...")
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	fromConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("create upgrade failed, err: %s\n", err.Error())
		return
	}
	defer fromConn.Close()

	toConn, err := dialBackendWsNode(r)
	if err != nil {
		fmt.Println("err: ", err)
		return
	}
	defer toConn.Close()

	proxyPass(fromConn, toConn)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	// if websocket
	if _, exists := r.Header["Upgrade"]; exists {
		fmt.Println("that's a websocket request")
		serveWs(w, r)
		return
	}

	fmt.Printf("current request is not a websocket request.\n")
	WriteHttpResponse(w, 200, -1, "not websocket request")
	return
}

func StartHttpProxy() {
	http.HandleFunc("/", Handler)
	addr := fmt.Sprintf(":%d", 8866)
	srv := &http.Server{
		Addr:         addr,
		Handler:      nil,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		fmt.Printf("ListenAndServe failed! err: %s\n", err.Error())
	}
}

func main() {
	go StartHttpProxy()

	fmt.Println("hey, http proxy start!")

	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case <- timer.C:
			timer.Reset(time.Second * 10)
			fmt.Println("hey, i am alive...")
		default:
		}
	}
}


type CommResponse struct {
	RetCode int 				`json:"RetCode"`
	RetMsg  string 				`json:"RetMsg"`
	Data 	*json.RawMessage 	`json:"Data,omitempty"`
}

func WriteHttpResponse(w http.ResponseWriter, httpRtnCode int ,retcode int, retmsg string) {
	w.WriteHeader(httpRtnCode)
	rsp := CommResponse{RetCode:retcode, RetMsg:retmsg,}
	bodyBytes, err := json.Marshal(rsp)
	if err != nil {
		fmt.Printf("json.Marshal(rsp) failed! err: %s", err.Error())
		return
	}
	buffer := bytes.NewBuffer(bodyBytes)
	io.Copy(w, buffer)
	return
}

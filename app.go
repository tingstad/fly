package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/gorilla/websocket"
)

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

var upgrader = websocket.Upgrader{}
var isLinux = false

func main() {
	if system := os.Getenv("OS"); system == "" {
		log.Fatal("Environment variable OS not set")
	} else {
		isLinux = system == "Linux"
	}
	port := "8080"

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]string{
			"Region": os.Getenv("FLY_REGION"),
		}
		_ = t.ExecuteTemplate(w, "index.html.tmpl", data)
	})
	http.HandleFunc("/ws", ws)

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

type writerFn func(p []byte) (int, error)

func (f writerFn) Write(p []byte) (n int, err error) {
	return f(p)
}

func ws(w http.ResponseWriter, r *http.Request) {
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrading:", err)
		return
	}
	defer func() {
		if err := wsConn.Close(); err != nil {
			log.Println("warn: closing ws:", err)
		}
	}()

	var args []string
	if isLinux {
		args = []string{"-qfc", "/dist/ninvaders", "/dev/null"}
	} else { // BSD
		args = []string{"-qF", "/dev/null", "ninvaders/nInvaders"}
	}
	cmd := exec.Command("script", args...)

	cmd.Stdout = writerFn(func(p []byte) (int, error) {
		msgType := websocket.TextMessage
		if err := wsConn.WriteMessage(msgType, p); err != nil {
			return 0, err
		} else {
			return len(p), nil
		}
	})

	cmdStdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Println("creating stdin pipe:", err)
		return
	}
	defer func() {
		_ = cmdStdin.Close()
	}()

	if err := cmd.Start(); err != nil {
		fmt.Println("starting command:", err)
		return
	}

	type message struct {
		typ  int
		data []byte
		err  error
	}

	quit := make(chan error, 1)
	input := make(chan message, 1)

	go func() {
		quit <- cmd.Wait()
	}()
	go func() {
		for {
			typ, data, err := wsConn.ReadMessage()
			input <- message{typ, data, err}
			if err != nil {
				log.Println("reading:", err)
				break
			}
		}
	}()

loop:
	for {
		select {
		case done := <-quit:
			log.Println("cmd done", done)
			break loop
		case msg := <-input:
			if _, err := cmdStdin.Write(msg.data); err != nil {
				log.Println("writing:", err)
				break loop
			}
		}
	}
	log.Println("finished")
}

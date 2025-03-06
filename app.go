package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

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
	defer func() { _ = cmdStdin.Close() }()

	quit := make(chan error)
	waitCmd := func() func() {
		var once sync.Once
		return func() {
			once.Do(func() {
				quit <- cmd.Wait()
				close(quit)
			})
		}
	}()
	if err := cmd.Start(); err != nil {
		fmt.Println("starting command:", err)
		return
	}
	defer func() {
		if !isRunning(cmd) {
			return
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err == nil {
			go waitCmd()
			select {
			case <-quit:
				return
			case <-time.After(2 * time.Second):
			}
		}
		_ = cmd.Process.Kill()
	}()

	type message struct {
		typ  int
		data []byte
		err  error
	}

	input := make(chan message, 1)

	go waitCmd()

	go func() {
		for {
			typ, data, err := wsConn.ReadMessage()
			input <- message{typ, data, err}
			if err != nil {
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
			if err := msg.err; err != nil {
				log.Println("reading:", err)
				break loop
			}
			if _, err := cmdStdin.Write(msg.data); err != nil {
				log.Println("writing:", err)
				break loop
			}
		}
	}
	log.Println("finished")
}

func isRunning(cmd *exec.Cmd) bool {
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return false
	}
	if cmd.ProcessState != nil && cmd.ProcessState.Success() {
		return false
	}
	return cmd.Process.Signal(syscall.Signal(0)) == nil
}

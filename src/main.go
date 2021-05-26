package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	tmpDir     = ""
	port       = ""
	status     = &statusStruct{cmdMap: make(map[*exec.Cmd]bool), tagMap: make(map[string]*tagStruct)}
	nrsc5      = ""
	lookFor    = []string{"Title", "Station name", "Slogan", "Artist", "Album", "Genre", "Audio bit rate", "BER", "MER", "Audio component"}
	mustStatOk = []string{page, gif, ico, catchup}
)

const (
	binName       = "yellowShoes"
	version       = binName + " Ver 1.09b"
	staticFs      = "../static"
	page          = staticFs + "/page.html"
	gif           = staticFs + "/wait.gif"
	ico           = staticFs + "/yellowShoes.jpg"
	catchup       = staticFs + "/128.wav"
	EXTN          = "wav"
	fileWaitConst = 42
	connMonitor   = 7
	tcpTimeout    = 5
	sig           = "SIG Service:"
	cmdWait       = 3
	seekDelta     = 8192
	lame          = "lame"
	deadBeat      = 50
)

type statusStruct struct {
	sync.RWMutex
	cmdMap           map[*exec.Cmd]bool
	tagMap           map[string]*tagStruct
	audioConnections int // connection count for audio stream
	messages         []string
	heartBeat        int64
}

type tagStruct struct {
	sync.RWMutex
	tag          string
	freq         string
	audioFile    string
	cmd          string
	pid          int
	programIndex string
	rtlTcp       string
	lookFor      []string
	infoMap      map[string]interface{}
	serviceSigs  map[string]int
	cmdPtr       *exec.Cmd
	done         bool
	goner        bool
	cmdExite     error
	isIOS        bool
	stdout       io.ReadCloser
}

func parseArgs() {
	argc := len(os.Args)
	switch argc {
	case 2:
		arg := os.Args[1]
		if strings.Contains(arg, "help") || arg == "h" || arg == "--h" || arg == "-h" || arg == "?" {
			showhelp()
			os.Exit(0)
		}
		if strings.Contains(arg, "version") || arg == "v" || arg == "--v" || arg == "-v" {
			fmt.Println("Version:", version)
			os.Exit(0)
		}
	case 5:
		for i, arg := range os.Args {
			if strings.Contains(arg, "-t") || strings.Contains(arg, "temp") {
				next := i + 1
				if argc > next {
					tmpDir = os.Args[next]
				}
			} else if strings.Contains(arg, "-p") || strings.Contains(arg, "port") {
				next := i + 1
				if argc > next {
					port = os.Args[next]
				}
				_, err := strconv.Atoi(port)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: Invalid port specified\n")
					os.Exit(1)
				}
			}
		}
	case 1:
		fmt.Println("Using defaults")
		tmpDir = os.TempDir()
		port = "8113"
	default:
		invalidUsage()
	}

	if tmpDir == "" || port == "" {
		invalidUsage()
	}

	writeTo := fmt.Sprintf("%s/%s.txt", tmpDir, nibble(8))
	writeThis := []byte("Test Write")
	err := ioutil.WriteFile(writeTo, writeThis, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: No write access to %s\n", tmpDir)
		os.Exit(1)
	}
	os.Remove(writeTo)
}

func nibble(span int) string {
	b := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	x := ""
	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)
	for i := 0; i < span; i++ {
		y := r1.Intn(len(b))
		x = fmt.Sprintf("%s%c", x, b[y])
	}
	return x
}

func showhelp() {
	fmt.Printf("Usage: %s -t /tmp -p 8118\n", os.Args[0])
	fmt.Println("  -h  --help         print this usage and exit")
	fmt.Println("  -t  --tempFolder   temp folder with write access to use")
	fmt.Println("  -p  --port         port to use")
	fmt.Println("  -v  --version      print version information and exit")
}

func invalidUsage() {
	fmt.Fprintf(os.Stderr, "Invalid usage\n")
	showhelp()
	os.Exit(1)
}

func main() {
	fmt.Printf("%s Copyright (C) Evuraan <evuraan@gmail.com>\nThis program comes with ABSOLUTELY NO WARRANTY.\n", version)
	parseArgs()
	fmt.Printf("Using temp dir: %s, port: %s\n", tmpDir, port)

	if checkExec("nrsc5") {
		nrsc5 = "nrsc5"
	} else if checkExec("nrsc5.exe") {
		nrsc5 = "nrsc5.exe"
	}
	if len(nrsc5) < 1 {
		fmt.Fprintf(os.Stderr, "Error 11.1 - Could not locate nrsc5 binary\n")
		os.Exit(1)
	}

	for i := range mustStatOk {
		item := mustStatOk[i]
		if !checkFile(item) {
			fmt.Fprintf(os.Stderr, "Error 11.2: %s does not exist\n", item)
			os.Exit(1)
		}
	}

	go status.init()
	mux := http.NewServeMux()
	mux.HandleFunc("/stop", status.stopAll)

	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/main", mainPageHandler)
	mux.HandleFunc("/favicon.ico", faviconHandler)
	mux.HandleFunc("/gif", gifHandler)
	mux.HandleFunc("/wav", wavHandler)
	mux.HandleFunc("/getStream", status.getStream)
	mux.HandleFunc("/getAudio", status.getAudio)
	mux.HandleFunc("/getInfo", status.getInfo)
	mux.HandleFunc("/whatsGoinOn", status.getActiveTag)
	mux.HandleFunc("/getErrMsg", status.getErrMsg)
	mux.HandleFunc("/getVersion", getVersion)
	mux.HandleFunc("/lameCheck", lameCheck)
	err := http.ListenAndServe(":"+port, mux)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen err %v\n", err)
		os.Exit(1)
	}
}

func (statusPtr *statusStruct) init() {
	self := statusPtr
	x := 0
	var delta int64
	for {
		self.RLock()
		x = self.audioConnections
		delta = time.Now().Unix() - self.heartBeat
		self.RUnlock()
		fmt.Println("Current Connections", x)
		if x == 0 && delta > deadBeat {
			go self.killAll()
		}
		time.Sleep(connMonitor * time.Second)
	}
}

func lameCheck(w http.ResponseWriter, r *http.Request) {
	if checkExec(lame) {
		fmt.Fprint(w, "OK")
	} else {
		fmt.Fprint(w, "No lame")
	}
}
func (statusPtr *statusStruct) getActiveTag(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "max-age=0")

	self := statusPtr
	self.RLock()
	defer self.RUnlock()

	if self.audioConnections == 0 {
		fmt.Fprint(w, "No_Active_Tags")
		return
	}

	for k := range self.tagMap {
		v := self.tagMap[k]
		stateMap := map[string]interface{}{"tag": v.tag, "freq": v.freq}
		data, err := json.Marshal(stateMap)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write(data)
			return
		} else {
			continue
		}
	}
	fmt.Fprint(w, "No_Active_Tags")
	return
}

func (statusPtr *statusStruct) killAll() bool {
	self := statusPtr
	x := 0

	newCmdMap := make(map[*exec.Cmd]bool)
	newTagMap := make(map[string]*tagStruct)

	self.Lock()
	oldCmdMap := self.cmdMap
	self.cmdMap = newCmdMap
	self.tagMap = newTagMap
	self.audioConnections = 0
	self.Unlock()

	for k := range oldCmdMap {
		go k.Process.Kill()
		x++
	}

	return (x > 0)
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, ico)
}
func gifHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, gif)
}

func wavHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "max-age=0")
	http.ServeFile(w, r, catchup)
}

func getVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "max-age=0")
	fmt.Fprint(w, version)
	return
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Incoming: GET Attempt from %s\n", r.RemoteAddr)
	http.Redirect(w, r, "./main", 301)
	return
}

func mainPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "max-age=0")
	http.ServeFile(w, r, page)
	return
}

func (statusPtr *statusStruct) stopAll(w http.ResponseWriter, r *http.Request) {
	go fmt.Printf("Stop all request from %s\n", r.RemoteAddr)
	if statusPtr == nil {
		return
	}
	go statusPtr.killAll()
	return
}

func checkFile(fileName string) bool {
	return (getFileSize(fileName) > 0)
}

func getFileSize(fileName string) int64 {
	fi, err := os.Stat(fileName)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func checkExec(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func runThis(cmdInPtr *string) error {
	if cmdInPtr == nil {
		errB := errors.New("cmdInPtr nil")
		return errB
	}
	cmdIn := *cmdInPtr
	fmt.Println("About to run", cmdIn)
	safetySplat := strings.Split(cmdIn, " ")
	cmdSplat := []string{}
	x := 0
	for i := range safetySplat {
		block := safetySplat[i]
		if block != "" {
			cmdSplat = append(cmdSplat, block)
			x++
		}
	}
	if x < 1 {
		err := errors.New("splat x 0")
		return err
	}

	cmd := exec.Command(cmdSplat[0], cmdSplat[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()

	if err == nil {
		fmt.Printf("bash pid: %v\n", cmd.Process.Pid)
		go cmd.Wait()
	}

	return err
}

func (tagPtr *tagStruct) run() error {

	self := tagPtr
	cmdIn := self.cmd
	fmt.Println("About to run", cmdIn)
	safetySplat := strings.Split(cmdIn, " ")
	cmdSplat := []string{}
	x := 0
	for i := range safetySplat {
		block := safetySplat[i]
		if block != "" {
			cmdSplat = append(cmdSplat, block)
			x++
		}
	}
	if x < 1 {
		err := errors.New("splat x 0")
		return err
	}

	cmd := self.cmdPtr
	cmd = exec.Command(cmdSplat[0], cmdSplat[1:]...)
	var err error
	self.stdout, err = cmd.StderrPipe()
	if err != nil {
		return err
	}
	err = cmd.Start()

	if err == nil {
		go func() {
			status.Lock()
			defer status.Unlock()
			status.cmdMap[cmd] = true
		}()
		go func() {
			self.cmdExite = cmd.Wait()
			fmt.Printf("cmd %s terminated: %v\n", self.cmd, self.cmdExite)
			status.Lock()
			defer status.Unlock()
			delete(status.tagMap, self.tag)
			delete(status.tagMap, self.freq)
			self.Lock()
			self.done = true
			self.Unlock()
			os.Remove(self.audioFile)
		}()

		go self.infoLoop()

	}

	return err
}

package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

func (statusPtr *statusStruct) getStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "max-age=0")
	qstrings := r.URL.Query()
	freq := qstrings.Get("freq")
	if len(freq) < 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	rtlTcp := qstrings.Get("rtl_tcp")
	if len(rtlTcp) < 1 {
		rtlTcp = "none"
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	rtlCheckStatus := false
	go checkRTL(rtlTcp, wg, &rtlCheckStatus)

	tag := nibble(16)

	self := statusPtr
	self.Lock()
	ourNewTag, ok := self.tagMap[freq]
	if !ok {
		ourNewTag = &tagStruct{freq: freq,
			tag:         tag,
			serviceSigs: map[string]int{},
			rtlTcp:      rtlTcp,
			lookFor:     lookFor, // a few items we need to find..
		}
		ourNewTag.infoMap = make(map[string]interface{})
		ourNewTag.infoMap["tag"] = tag
	}
	self.messages = nil
	self.messages = []string{}
	self.tagMap[freq] = ourNewTag
	self.tagMap[tag] = ourNewTag
	self.Unlock()

	ourNewTag.audioFile = fmt.Sprintf("%s/%s.%s", tmpDir, tag, EXTN)

	ourNewTag.programIndex = "0"
	ourNewTag.programIndex = qstrings.Get("program")
	if len(ourNewTag.programIndex) < 1 {
		ourNewTag.programIndex = "0"
	}

	ourNewTag.cmd = fmt.Sprintf("%s %s %s -o %s", nrsc5, freq, ourNewTag.programIndex, ourNewTag.audioFile)
	if ourNewTag.rtlTcp != "none" {
		ourNewTag.cmd = fmt.Sprintf("%s -H %s", ourNewTag.cmd, ourNewTag.rtlTcp)
	}

	if checkExec("stdbuf") {
		ourNewTag.cmd = fmt.Sprintf("stdbuf -oL %s", ourNewTag.cmd)
	}

	wg.Wait()
	if !rtlCheckStatus {
		msg := fmt.Sprintf("Error: could not reach your rtltcp-host: %s", rtlTcp)
		fmt.Fprint(w, msg)
		fmt.Println(msg)
		return
	}

	err := ourNewTag.run()
	if err == nil {
		fmt.Fprintf(w, "%s", tag)
	} else {
		w.WriteHeader(http.StatusBadGateway)
	}
	return
}

func checkRTL(rtlInfo string, wg *sync.WaitGroup, rtlCheckStatusPtr *bool) bool {
	defer wg.Done()

	if len(rtlInfo) < 1 {
		return false
	}

	if rtlInfo == "none" {
		*rtlCheckStatusPtr = true
		return true
	}
	splat := strings.Split(rtlInfo, ":")

	if len(splat) < 2 {
		rtlInfo += ":1234"
	}
	conn, err := net.DialTimeout("tcp", rtlInfo, tcpTimeout*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	x := (err == nil)
	*rtlCheckStatusPtr = x
	return x
}

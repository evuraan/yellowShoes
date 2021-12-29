package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
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

	userAgent := r.UserAgent()
	isIOSDevice := strings.Contains(userAgent, "iPhone") || strings.Contains(userAgent, "iPod") || strings.Contains(userAgent, "iPad") // || strings.Contains(userAgent, "Chrome")
	if isIOSDevice {
		ourNewTag.isIOS = true
	} else {
		codec := qstrings.Get("format")
		ourNewTag.isIOS = (codec == "mp3")
	}

	ourNewTag.audioFile = fmt.Sprintf("%s/%s.%s", tmpDir, tag, EXTN)
	if ourNewTag.isIOS {
		ourNewTag.audioFile = fmt.Sprintf("%s/%s.%s", tmpDir, tag, "mp3")
	}

	ourNewTag.programIndex = "0"
	ourNewTag.programIndex = qstrings.Get("program")
	if len(ourNewTag.programIndex) < 1 {
		ourNewTag.programIndex = "0"
	}

	stdbuf := checkExec("stdbuf")

	wg.Wait()
	if !rtlCheckStatus {
		msg := fmt.Sprintf("Error: could not reach your rtltcp-host: %s", rtlTcp)
		fmt.Fprint(w, msg)
		fmt.Println(msg)
		return
	}

	// push the reaper a bit away
	go self.beepBoop()

	giveTag := false

	if !ourNewTag.isIOS {
		ourNewTag.cmd = fmt.Sprintf("%s %s %s -o %s", nrsc5, freq, ourNewTag.programIndex, ourNewTag.audioFile)
		if ourNewTag.rtlTcp != "none" {
			ourNewTag.cmd = fmt.Sprintf("%s -H %s", ourNewTag.cmd, ourNewTag.rtlTcp)
		}

		if stdbuf {
			ourNewTag.cmd = fmt.Sprintf("stdbuf -oL %s", ourNewTag.cmd)
		}

		err := ourNewTag.run()
		if err == nil {
			giveTag = true
		} else {
			w.WriteHeader(http.StatusBadGateway)
		}
	} else {
		// stdbuf -oL nrsc5 88.5 0 -H 192.168.1.134 -o - | lame - --preset insane  moha.mp3
		if !checkExec(lame) {
			msg := "Error: Could not find mp3 encoder\n"
			fmt.Fprintf(os.Stderr, msg)
			fmt.Fprint(w, msg)
			return
		}
		var lhs, rhs *exec.Cmd
		if stdbuf {
			lhs = exec.Command("stdbuf", "-oL", nrsc5, freq, ourNewTag.programIndex, "-o", "-")
			if ourNewTag.rtlTcp != "none" {
				lhs = exec.Command("stdbuf", "-oL", nrsc5, freq, ourNewTag.programIndex, "-o", "-", "-H", ourNewTag.rtlTcp)
			}
		} else {
			lhs = exec.Command(nrsc5, freq, ourNewTag.programIndex, "-o", "-")
			if ourNewTag.rtlTcp != "none" {
				lhs = exec.Command(nrsc5, freq, ourNewTag.programIndex, "-o", "-", "-H", ourNewTag.rtlTcp)
			}
		}
		rhs = exec.Command("lame", "-V", "0", "-", ourNewTag.audioFile)
		// rhs = exec.Command("lame", "--preset", "insane", "-", ourNewTag.audioFile)
		r, wr := io.Pipe()
		lhs.Stdout = wr
		rhs.Stdin = r

		var err error
		ourNewTag.stdout, err = lhs.StderrPipe()
		if err != nil {
			fmt.Println("lhs set stdout err", err)
			return
		}

		err = lhs.Start()

		if err == nil {
			go rhs.Run()
			go func() {
				status.Lock()
				defer status.Unlock()
				status.cmdMap[lhs] = true
			}()

			go func() {
				ourNewTag.cmdExite = lhs.Wait()
				fmt.Println("lhs ended:", ourNewTag.cmdExite)
				wr.Close()
				r.Close()
				status.Lock()
				defer status.Unlock()
				delete(status.tagMap, ourNewTag.tag)
				delete(status.tagMap, ourNewTag.freq)
				ourNewTag.Lock()
				ourNewTag.done = true
				ourNewTag.Unlock()
				os.Remove(ourNewTag.audioFile)
			}()

			go ourNewTag.infoLoop()
			giveTag = true

		} else {
			fmt.Printf("lhs start err %v\n", err)
			return
		}

	}

	if giveTag {
		fmt.Fprintf(w, "%s", tag)
		go self.beepBoop()
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

	return portCheck(rtlInfo)
}

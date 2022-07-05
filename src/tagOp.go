package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func (statusPtr *statusStruct) clientGone() {
	statusPtr.Lock()
	defer statusPtr.Unlock()
	statusPtr.audioConnections--
	if statusPtr.audioConnections < 0 {
		statusPtr.audioConnections = 0
	}
}

func (statusPtr *statusStruct) newClient() {
	statusPtr.Lock()
	defer statusPtr.Unlock()
	if statusPtr.audioConnections < 0 {
		statusPtr.audioConnections = 0
	}
	statusPtr.audioConnections++
}

func (statusPtr *statusStruct) beepBoop() {
	statusPtr.Lock()
	defer statusPtr.Unlock()
	statusPtr.heartBeat = time.Now().Unix()
}

func (ourTag *tagStruct) isGone() bool {
	ourTag.RLock()
	defer ourTag.RUnlock()
	return ourTag.goner
}

func (ourTag *tagStruct) isDone() bool {
	ourTag.RLock()
	defer ourTag.RUnlock()
	return ourTag.done

}

func (ourTag *tagStruct) setGonerGone() bool {
	ourTag.Lock()
	defer ourTag.Unlock()
	ourTag.goner = true
	return true
}

func (ourTag *tagStruct) isTagGone() bool {
	ourTag.RLock()
	defer ourTag.RUnlock()
	return ourTag.goner
}

func (statusPtr *statusStruct) getAudio(w http.ResponseWriter, r *http.Request) {
	go fmt.Printf("getAudio req from client %s\n", r.RemoteAddr)

	w.Header().Set("Cache-Control", "max-age=0")
	qstrings := r.URL.Query()
	tag := qstrings.Get("tag")
	if len(tag) < 1 {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	self := statusPtr
	self.RLock()
	ourTag, ok := self.tagMap[tag]
	self.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	ourTag.RLock()
	err := ourTag.cmdExite
	ourTag.RUnlock()

	if err != nil {
		fmt.Fprintf(w, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	audioFile := ourTag.audioFile
	go self.newClient()

	defer func() {
		fmt.Printf("Conn exit getAudio for %v\n", r.RemoteAddr)
		go ourTag.setGonerGone()
		go self.clientGone()
	}()

	for i := 0; i < fileWaitConst; i++ {
		fmt.Printf("getAudio: Waiting for %s\n", audioFile)
		if getFileSize(audioFile) > 10 {
			break
		}
		if ourTag.isGone() {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !ourTag.isIOS {
		if !fixWav(audioFile) {
			fmt.Fprintf(os.Stderr, "Error - could not tweak %s\n", audioFile)
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		w.Header().Set("Content-Type", "audio/x-wav")
	} else {
		w.Header().Set("Content-Type", "audio/mpeg")
	}
	source, err := os.Open(audioFile)
	if err != nil {
		fmt.Println("got err", err)
		return
	}
	var pos, delta int64
	lastPos := pos
	stale := 0
	blank := 0
	loopCount := 0

	for {
		pos, err = source.Seek(0, 2) // Seek to End
		if err != nil {
			fmt.Println("Seek err", err)
			return
		}

		if loopCount == 0 && !ourTag.isIOS {
			if pos > seekDelta {
				func() {
					vex, err := os.Open(catchup)
					if err == nil {
						defer vex.Close()
						n, err := io.Copy(w, vex)
						if err == nil {
							go self.beepBoop()
							fmt.Printf("Catchup %d bytes sent\n", n)
							loopCount++
							lastPos = pos - seekDelta
						}
					}
				}()
			}
			// else {
			// 	var x int64 = 8192
			// 	if pos > x {
			// 		lastPos = pos - x
			// 	}
			// }
		}

		delta = pos - lastPos

		if delta > 0 {
			peg, errSeek := source.Seek(lastPos, 0)
			if errSeek != nil {
				fmt.Println("errSeek", errSeek)
				return
			}
			_ = peg

			lastPos = pos
			n, err := io.Copy(w, source)
			if err != nil {
				fmt.Println("Copy err", err)
				return
			}
			go self.beepBoop()
			go fmt.Printf("Sent %d bytes from %d to %v\n", n, peg, r.RemoteAddr)
			blank++
			stale = 0
		} else {
			stale++
			if blank == 0 {
				time.Sleep(3 * time.Second)
			} else {
				time.Sleep(1 * time.Second)
			}
		}

		if stale > 30 {
			return
		}
		loopCount++
	}

}

func (statusPtr *statusStruct) getErrMsg(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "max-age=0")
	statusPtr.RLock()
	defer statusPtr.RUnlock()
	if len(statusPtr.messages) > 0 {
		fmt.Fprint(w, strings.Join(statusPtr.messages[:], "\n"))
	}
}

func (statusPtr *statusStruct) getInfo(w http.ResponseWriter, r *http.Request) {
	qstrings := r.URL.Query()
	tag := qstrings.Get("tag")
	if len(tag) < 1 {
		return
	}
	self := statusPtr

	self.RLock()
	ourTag, ok := self.tagMap[tag]
	self.RUnlock()
	if !ok {
		return
	}
	ourTag.Lock()
	ourTag.infoMap["SIGCT"] = len(ourTag.serviceSigs)
	ourTag.infoMap["freq"] = ourTag.freq
	ourTag.infoMap["programIndex"] = ourTag.programIndex
	ourTag.infoMap["Format"] = "wav"
	if ourTag.isIOS {
		ourTag.infoMap["Format"] = "mp3"
	}
	data, err := json.Marshal(ourTag.infoMap)
	ourTag.Unlock()

	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=0")
		w.Write(data)
	}
}

func (statusPtr *statusStruct) appendMessage(message string) {
	self := statusPtr
	if len(message) < 1 {
		return
	}
	self.Lock()
	defer self.Unlock()
	self.messages = append(self.messages, message)
}

func (tagPtr *tagStruct) infoLoop() {
	fmt.Println("Called infoloop")
	self := tagPtr
	m := ""

	scanner := bufio.NewScanner(self.stdout)
	for scanner.Scan() {
		m = scanner.Text()
		go status.appendMessage(m)
		fmt.Println("mmmm", m)
		for i := range self.lookFor {
			v := self.lookFor[i]
			if strings.Contains(m, v) {
				splat := strings.Split(m, v+":")
				if len(splat) != 2 {
					continue
				}
				if len(splat[1]) < 3 {
					continue
				}
				go func() {
					self.Lock()
					self.infoMap[v] = strings.TrimSpace(splat[1])
					self.Unlock()
				}()
			} else if strings.Contains(m, sig) {
				splat := strings.Split(m, sig)
				if len(splat) != 2 {
					continue
				}
				if len(splat[1]) < 3 {
					continue
				}
				go func() {
					self.Lock()
					self.serviceSigs[splat[1]] = len(self.serviceSigs)
					self.Unlock()
				}()
			}
		}
	}
}

package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"github.com/lxn/win"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Speaker struct {
	note   int
	volume int
}
type Cmd struct {
	cmd     byte
	delay   int
	channel int
	data1   int
	data2   int
}

type TrackProgress struct {
	cmdCompleted int
	currentDelay int
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

var tracks [][]Cmd
var speakers []Speaker
var bpm = 120
var mpq = 500000
var tpq = 48

func main() {
	fmt.Println("File To Load:")
	nameReader := bufio.NewReader(os.Stdin)
	fName, _ := nameReader.ReadString('\n')
	fName = strings.TrimRight(fName, "\r\n")
	fName = "E:/music/" + fName + ".mid"
	fmt.Println("Loading: " + fName)
	f, err := os.Open(fName)
	check(err)
	r := bufio.NewReaderSize(f, 100000)
	tracks = ParseMidi(r)
	fmt.Printf("Done Parsing Midi\n")
	speakerCount := 5
	speakers = make([]Speaker, 0)
	for i := 0; i < speakerCount; i++ {
		speakers = append(speakers, Speaker{note: 0, volume: 0})
	}

	//speakers = append(speakers, Speaker{note: 0, volume: 0})
	conn, err := net.Dial("tcp", "192.168.0.50:80")
	if err != nil {
		fmt.Println(err)
	}
	go HandleReceiveData(conn)
	for {
		fmt.Printf("Running main still: %d\n", time.Now)
		time.Sleep(time.Second)
	}
}

func AllTracksCompleted(trackProg []TrackProgress) bool {
	for i := 0; i < len(tracks); i++ {
		//fmt.Printf("done: %d, total: %d\n", trackProg[i].cmdCompleted, len(tracks[i]))
		if trackProg[i].cmdCompleted < len(tracks[i]) {
			return false
		}
	}
	return true
}
func SendTrackWifi(conn net.Conn) {
	w := bufio.NewWriter(conn)
	fmt.Printf("Using %d speakers.\n", len(speakers))

	//trackToPlay := tracks[1]
	//tracks = [][]Cmd{tracks[0], tracks[2], tracks[3]}
	trackProg := make([]TrackProgress, len(tracks))
	/*
		for trackToPlay[0].cmd != 0x90 {
			trackToPlay = append(trackToPlay[:0], trackToPlay[1:]...)
		}
	*/
	for {
		//fmt.Printf("Looping\n")
		//fmt.Printf("Done Tick\n")
		if AllTracksCompleted(trackProg) {
			fmt.Printf("All Tracks Done\n")
			return
		}
		//fmt.Printf("Passed Completion check\n")
		for i := 0; i < len(tracks); i++ {
			//fmt.Printf("In len loop\n")
			for {
				if trackProg[i].cmdCompleted >= len(tracks[i]) {
					break
				}
				if trackProg[i].currentDelay < tracks[i][trackProg[i].cmdCompleted].delay {
					trackProg[i].currentDelay++
					break
				}
				//Stop note cmd
				if tracks[i][trackProg[i].cmdCompleted].cmd == 0x51 {
					mpq = tracks[i][trackProg[i].cmdCompleted].data1
				}
				if tracks[i][trackProg[i].cmdCompleted].cmd == 0x80 {
					for s := 0; s < len(speakers); s++ {
						if speakers[s].note == tracks[i][trackProg[i].cmdCompleted].data1 {
							fmt.Printf("Stopping: %d\n", tracks[i][trackProg[i].cmdCompleted].data1)
							speakers[s].volume = 0
							w.Write([]byte("0,0," + strconv.Itoa(s) + ",."))
							w.Flush()
							break
						}
					}
				}
				if tracks[i][trackProg[i].cmdCompleted].cmd == 0x90 {
					if tracks[i][trackProg[i].cmdCompleted].data2 == 0 {
						//Turn off speaker with this data1
						for s := 0; s < len(speakers); s++ {
							if speakers[s].note == tracks[i][trackProg[i].cmdCompleted].data1 {
								fmt.Printf("Stopping: %d\n", tracks[i][trackProg[i].cmdCompleted].data1)
								speakers[s].volume = 0
								w.Write([]byte("0,0," + strconv.Itoa(s) + ",."))
								w.Flush()
								break
							}
						}
					} else {
						//Find open speaker and use it
						for s := 0; s < len(speakers); s++ {
							if speakers[s].volume == 0 {
								var freq = int(27.5 * math.Pow(2, float64(tracks[i][trackProg[i].cmdCompleted].data1-21)/12))
								sc := strconv.Itoa(freq) + "," + strconv.Itoa(tracks[i][trackProg[i].cmdCompleted].data2) + "," + strconv.Itoa(s) + ",."
								fmt.Printf("Sending: %s -- note %d\n", sc, tracks[i][trackProg[i].cmdCompleted].data1)
								speakers[s].volume = tracks[i][trackProg[i].cmdCompleted].data2
								speakers[s].note = tracks[i][trackProg[i].cmdCompleted].data1
								w.Write([]byte(sc))
								w.Flush()
								break
							}
						}
					}
				}
				trackProg[i].currentDelay = 0
				trackProg[i].cmdCompleted++
			}

		}
		time.Sleep(time.Microsecond * time.Duration(mpq/tpq))
	}

}
func HandleReceiveData(conn net.Conn) {
	donePlay := false
	r := bufio.NewReader(conn)
	defer conn.Close()
	line := ""
	for {
		//msg := make([]byte, 1024)
		s, readErr := r.ReadByte()
		if readErr != nil {
			fmt.Println("Err: ", readErr)
			conn.Close()
			os.Exit(1)
		}
		if s == '\n' {
			if line == "play" {
				if donePlay == false {
					go SendTrackWifi(conn)
					donePlay = true
				}
			}
			fmt.Printf(string(line) + "\n")
		} else {
			line += string(s)
		}
	}
}

var wg sync.WaitGroup

func ParseMidi(r *bufio.Reader) [][]Cmd {
	header := make([]byte, 14)
	_, err := r.Read(header)
	check(err)
	format := binary.BigEndian.Uint16(header[8:10])
	trackCount := binary.BigEndian.Uint16(header[10:12])
	newTpq := binary.BigEndian.Uint16(header[12:cap(header)])
	//fmt.Printf("Read %d bytes: %s", n, string(header))
	tpq = int(newTpq)
	fmt.Printf("Format: %d, count: %d, tpq: %d\n", format, trackCount, newTpq)
	tracks := make([][]Cmd, trackCount)
	for i := 0; i < int(trackCount); i++ {
		trackHeader := make([]byte, 8)
		_, err = r.Read(trackHeader)
		check(err)
		trackLen := binary.BigEndian.Uint32(trackHeader[4:8])
		trackContent := make([]byte, trackLen)
		ret, err := r.Read(trackContent)
		check(err)
		if ret != int(trackLen) {
			fmt.Printf("Err on track %d, failed to read entire track. read: %d\n", i, ret)
		}
		if i == 5 {
			wg.Add(1)
			go ParseTrack(trackContent, &tracks[i], true)
		} else {
			wg.Add(1)
			go ParseTrack(trackContent, &tracks[i], false)
		}
	}
	wg.Wait()
	/*
		longest := 0
		longestIndex := 0
		for i := 0; i < len(tracks); i++ {
			fmt.Printf("Track Len: %d\n", len(tracks[i]))
			if len(tracks[i]) > longest {
				longest = len(tracks[i])
				longestIndex = i
			}
		}
		fmt.Printf("Longest Track: %d\n", longestIndex)
		PlayTrack(tracks[6])
	*/
	return tracks
}
func PrintTrack(track []Cmd) {
	for i := 0; i < len(track); i++ {
		fmt.Printf("Cmd: %x, Delay: %d, Data: %d, Data2: %d\n", track[i].cmd, track[i].delay, track[i].data1, track[i].data2)
	}
}
func PlayTrack(track []Cmd) {
	for track[0].cmd != 0x90 {
		track = append(track[:0], track[1:]...)
	}
	for len(track) != 0 {
		fmt.Printf("Doing Cmd: %x, Delay: %d, Data: %d, Data2: %d, Remaning: %d\n", track[0].cmd, track[0].delay, track[0].data1, track[0].data2, len(track))
		if track[0].cmd == 0x90 && track[0].data2 > 0 {
			_ = win.MessageBeep(win.MB_OK)
		}
		track = append(track[:0], track[1:]...)

		if len(track) > 0 {
			fmt.Printf("Coming Delay: %d\n", track[0].delay)
			time.Sleep(time.Duration(track[0].delay) * time.Millisecond * 5)
		}
	}
}
func ParseTrack(tr []byte, cmdPointer *[]Cmd, debug bool) {
	cmdList := make([]Cmd, 0)
	defer wg.Done()
	var curByte int
	var lastCommand byte
	for {
		var deltaTime int = 0
		deltaTimeBytes := make([]byte, 0)
		for tr[curByte]&128 > 0 {
			//fmt.Printf("%08b\n", int64(tr[curByte]))
			deltaTimeBytes = append(deltaTimeBytes, tr[curByte]&127)
			curByte++
		}
		//fmt.Printf("%08b\n", int64(tr[curByte]))
		deltaTimeBytes = append(deltaTimeBytes, tr[curByte])
		curByte++
		for i := 0; i < len(deltaTimeBytes); i++ {
			deltaTime += int(deltaTimeBytes[i]) * int(math.Pow(float64(128), float64(len(deltaTimeBytes)-i-1)))
			if debug {
				//fmt.Printf("Multiplier: %d\n", int(math.Pow(float64(128), float64(len(deltaTimeBytes)-i-1))))
			}
		}

		event := tr[curByte]

		if int(event) >= 128 {
			lastCommand = event
			curByte++
		} else {
			event = lastCommand
		}

		if int(event) == 0xFF {
			//Handle Meta Event
			cmd := tr[curByte]
			if cmd == 0x2F {
				return
			}

			curByte++
			n := tr[curByte]
			curByte++

			data := tr[curByte : curByte+int(n)]
			curByte += int(n)
			if debug {
				if false {

					fmt.Printf("Meta Command: %x\n", cmd)
					fmt.Printf("  At time: %d\n", deltaTime)
					fmt.Printf("  Data Len: %x\n", n)
					fmt.Printf("  Data: %s\n", data)
				}

			}
			var newCmd Cmd
			if cmd == 0x51 {
				t := int(uint(data[2]) | uint(data[1])<<8 | uint(data[0])<<16)
				newCmd = Cmd{cmd: 0x51, delay: deltaTime, channel: 0, data1: t, data2: int(0)}
			} else {
				newCmd = Cmd{cmd: 0x00, delay: deltaTime, channel: 0, data1: int(0), data2: int(0)}
			}
			*cmdPointer = append(cmdList, newCmd)
			cmdList = *cmdPointer
		} else if int(event) < 128 {
			//Handle running event???
			return
		} else {
			//handle normal event

			cmd := event & 0xF0
			var chn = int(event&1) + int(event&2) + int(event&4) + int(event&8)
			data := tr[curByte]
			var data2 byte
			curByte++
			var hasMoreData = true
			if cmd == 0xC0 || cmd == 0xD0 {
				hasMoreData = false
			} else {
				data2 = tr[curByte]
				curByte++
			}

			if debug {
				if cmd == 0x80 || cmd == 0x90 {
					if false {
						fmt.Printf("Normal Command: %x, %08b\n", event, event)
						fmt.Printf("  Cmd: %x\n", cmd)
						fmt.Printf("  At time: %d\n", deltaTime)
						fmt.Printf("  chan: %d\n", chn)
						fmt.Printf("  data: %08b\n", data)
						if hasMoreData {
							fmt.Printf("  data2: %08b\n", data2)
						}

						fmt.Printf("Adding to list\n")
					}
				}
			}

			if cmd == 0x80 || cmd == 0x90 {
				newCmd := Cmd{cmd: cmd, delay: deltaTime, channel: chn, data1: int(data), data2: int(data2)}
				*cmdPointer = append(cmdList, newCmd)
				cmdList = *cmdPointer
			} else {
				newCmd := Cmd{cmd: 0x00, delay: deltaTime, channel: chn, data1: int(0), data2: int(0)}
				*cmdPointer = append(cmdList, newCmd)
				cmdList = *cmdPointer
			}
		}

	}
	if debug {
		fmt.Printf("Naturally Closed at %d\n", curByte)
	}
}

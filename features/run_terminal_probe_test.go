package features

import (
	"path/filepath"
	"testing"
)

func TestTerminalProbeBuild(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if err := recoveryBuildHelper(root, filepath.Join(t.TempDir(), "slice-37-probe"), terminalProbeSource); err != nil {
		t.Fatal(err)
	}
}

// Compiled as a trusted fixture; executed only inside the guarded CI worker.
const terminalProbeSource = `package main
import (
 "encoding/json"
 "fmt"
 "io"
 "os"
 "os/signal"
 "strconv"
 "strings"
 "syscall"
 "time"
 "golang.org/x/term"
 "golang.org/x/sys/unix"
)
type report struct {
 TTY [3]bool
 Initial [2]int
 Sizes [][2]int
 Args, Environment []string
 UID,GID int
 Workspace string
 Input []byte
 Signals []string
}
func save(r report) {
 data,err:=json.Marshal(r);if err!=nil{panic(err)}
 if err=os.WriteFile("/workspace/slice-37-report.next",data,0600);err!=nil{panic(err)}
 if err=os.Rename("/workspace/slice-37-report.next","/workspace/slice-37-report.json");err!=nil{panic(err)}
}
func main(){
 if len(os.Args)<3{os.Exit(96)}
 mode:=os.Args[1];exit,err:=strconv.Atoi(os.Args[2]);if err!=nil{panic(err)}
 columns,rows,err:=term.GetSize(0);if err!=nil{panic(err)}
 cwd,err:=os.Getwd();if err!=nil{panic(err)}
 r:=report{TTY:[3]bool{term.IsTerminal(0),term.IsTerminal(1),term.IsTerminal(2)},Initial:[2]int{columns,rows},Args:os.Args[3:],UID:os.Geteuid(),GID:os.Getegid(),Workspace:cwd,Environment:os.Environ()}
 if mode=="canonical"{
  state,err:=unix.IoctlGetTermios(0,unix.TCGETS);if err!=nil{panic(err)}
  state.Lflag|=unix.ICANON|unix.ISIG;state.Lflag &^= unix.ECHO;state.Oflag &^= unix.OPOST
  if err=unix.IoctlSetTermios(0,unix.TCSETS,state);err!=nil{panic(err)}
 }else{if _,err=term.MakeRaw(0);err!=nil{panic(err)}}
 signals:=make(chan os.Signal,16);signal.Notify(signals,syscall.SIGINT,syscall.SIGTERM,syscall.SIGWINCH)
 if err=os.WriteFile("/workspace/slice-37-worker-edit",[]byte("slice-37 retained edit"),0600);err!=nil{panic(err)}
 save(r)
 fmt.Fprint(os.Stdout,"slice-37:ready\n")
 if mode=="argv"{
  os.Stdout.Write([]byte("slice-37 stdout\n\r\n\r\x01\x00\x00\x00\x00\x00\x00\x03raw\xff"))
  fmt.Fprint(os.Stderr,"slice-37 stderr")
  os.Exit(exit)
 }
 input:=make(chan []byte,1)
 if mode!="ignore" && mode!="handle" {
  go func(){for {b:=make([]byte,128);n,err:=os.Stdin.Read(b);if n>0{input<-b[:n]};if err!=nil {if err!=io.EOF{panic(err)};return}}}()
 }
 for {
  select {
  case s:=<-signals:
   if s==syscall.SIGWINCH{c,h,err:=term.GetSize(0);if err!=nil{panic(err)};r.Sizes=append(r.Sizes,[2]int{c,h});save(r);continue}
   name:="SIGINT";if s==syscall.SIGTERM{name="SIGTERM"};r.Signals=append(r.Signals,name);save(r)
   if mode=="handle"{time.Sleep(100*time.Millisecond);os.Exit(exit)}
  case b:=<-input:
   r.Input=append(r.Input,b...);save(r)
   if strings.HasSuffix(string(r.Input),"release\n"){os.Exit(exit)}
  }
 }
}
`

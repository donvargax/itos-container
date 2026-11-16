package features

// The harness compiles this trusted probe; only the sandbox executes it. The
// copied ELF is started directly, with no shell interpreter concealing noexec.
const homeNativeProbeSource = `package main
import (
    "encoding/json"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "syscall"
)
type report struct {
    UID, GID, RootUID, RootGID int
    Home, Tmp, Mode string
    Writable, HomeBefore, TmpBefore, TmpExec, HostReadable bool
    Initial []string
    Errors []string
}
func main() {
    if len(os.Args) == 2 && os.Args[1] == "elf" {
        fmt.Print("slice-34-elf-executed")
        return
    }
    if len(os.Args) != 3 { os.Exit(99) }
    r := report{UID:os.Geteuid(), GID:os.Getegid(), Home:os.Getenv("HOME"), Tmp:os.Getenv("TMPDIR"), Initial:[]string{}, Errors:[]string{}}
    check := func(err error) { if err != nil { r.Errors=append(r.Errors, err.Error()) } }
    info, err := os.Lstat(r.Home)
    check(err)
    if err == nil {
        r.Mode=info.Mode().String()
        if st, ok := info.Sys().(*syscall.Stat_t); ok { r.RootUID=int(st.Uid); r.RootGID=int(st.Gid) }
    }
    entries, err := os.ReadDir(r.Home)
    check(err)
    for _, entry := range entries { r.Initial=append(r.Initial, entry.Name()) }
    marker := filepath.Join(r.Home,"slice-34-home-marker")
    tmp := "/tmp/slice-34-tmp-marker"
    data, err := os.ReadFile(marker)
    r.HomeBefore=err==nil && string(data)=="retained"
    _, err = os.ReadFile(tmp)
    r.TmpBefore=err==nil
    _, err = os.ReadFile(os.Args[2])
    r.HostReadable=err==nil
    if os.Args[1]=="write" {
        err=os.WriteFile(marker,[]byte("retained"),0600)
        r.Writable=err==nil
        check(err)
    } else {
        f, err:=os.CreateTemp(r.Home,"slice-34-write-")
        r.Writable=err==nil
        check(err)
        if err==nil { name:=f.Name(); check(f.Close()); check(os.Remove(name)) }
    }
    check(os.WriteFile(tmp,[]byte("temporary"),0600))
    data,err=os.ReadFile("/workspace/slice-34-probe")
    check(err)
    if err==nil {
        path:="/tmp/slice-34-probe"
        check(os.WriteFile(path,data,0700))
        output,err:=exec.Command(path,"elf").CombinedOutput()
        check(err)
        r.TmpExec=err==nil && string(output)=="slice-34-elf-executed"
    }
    check(json.NewEncoder(os.Stdout).Encode(r))
    // Keep the worker alive for independent native mount inspection, then let
    // the product observe exit and remove its session resources.
    _,err=io.Copy(io.Discard,os.Stdin)
    if err!=nil { os.Exit(98) }
}
`

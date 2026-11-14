package port

// Executable is one read-only observation of a program file. Path is its
// resolved absolute path, every symbolic link followed. The other fields read
// its ELF headers and are zero for a file that is not ELF. Nothing is executed.
type Executable struct {
	Path string
	// Regular is whether the resolved path names a regular file.
	Regular bool
	// OS and Architecture are "linux" and "amd64" for a 64-bit x86-64 ELF in
	// the System V or Linux ABI, and anything else, or empty, otherwise.
	OS, Architecture string
	// Interpreter is a PT_INTERP segment. Dynamic is a dynamic segment or
	// section, with or without a needed library.
	Interpreter, Dynamic bool
}

// Executables observes the controller's own program: the file the process was
// started from, never PATH or a name the checkout supplies. A failure is an
// observation that could not be made, never one to read as safe.
type Executables interface {
	Observe() (Executable, Failure)
}

package executable

import "os"

// Windows has no pipe a path can name in a program's place.
const openFlags = os.O_RDONLY

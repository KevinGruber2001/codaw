// bridge.c compiles the miniaudio implementation ONCE for the whole program.
//
// miniaudio.h is a "single-header" library: by default it only declares
// functions (like a normal .h). Defining MINIAUDIO_IMPLEMENTATION before the
// include flips it into "give me the actual function bodies" mode. That must
// happen in exactly one .c file in the build, otherwise you get duplicate-symbol
// linker errors. This file is that one place.
//
// audio.go includes the same header WITHOUT the define, so it only sees the
// declarations and links against the symbols compiled here.

#define MINIAUDIO_IMPLEMENTATION

#include "miniaudio.h"

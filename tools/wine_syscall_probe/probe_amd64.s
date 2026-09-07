#include "textflag.h"

// func rawGetpid() uint64
TEXT ·rawGetpid(SB), NOSPLIT, $0-8
    MOVQ $39, AX   // Linux x86-64 syscall number for getpid
    SYSCALL
    MOVQ AX, ret+0(FP)
    RET

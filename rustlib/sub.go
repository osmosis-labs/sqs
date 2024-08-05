package rustexports

/*
#include "../target/release/librustlib.h"
*/
import "C"

func Sub(a, b float64) float64 {
	result := C.sub(C.double(a), C.double(b))
	return float64(result)
}

package rustexports

/*
#include "../target/release/librustlib.h"
*/
import "C"

func Add(a, b float64) float64 {
	result := C.add(C.double(a), C.double(b))
	return float64(result)
}

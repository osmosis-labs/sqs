mod numbers;
mod result;
mod transmuter;

use crate::result::FFIResult;
use transmuter::FFIDivision;

#[no_mangle]
pub extern "C" fn print_division(division: FFIDivision) -> FFIResult<u8> {
    println!("{:?}", division.into_division());
    FFIResult::ok(0)
}

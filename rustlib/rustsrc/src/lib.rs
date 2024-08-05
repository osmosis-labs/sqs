#[no_mangle]
pub extern "C" fn add(left: f64, right: f64) -> f64 {
    left + right
}

#[no_mangle]
pub extern "C" fn sub(left: f64, right: f64) -> f64 {
    left - right
}

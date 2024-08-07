pub fn nullable_ptr_to_option<T: Clone>(ptr: *const T) -> Option<T> {
    if ptr.is_null() {
        None
    } else {
        Some(unsafe { &*ptr }.clone())
    }
}

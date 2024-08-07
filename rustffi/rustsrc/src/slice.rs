#[repr(C)]

pub struct FFISlice<T> {
    ptr: *const T,
    len: usize,
}

impl<T> FFISlice<T> {
    pub fn as_slice(&self) -> &[T] {
        unsafe { std::slice::from_raw_parts(self.ptr, self.len) }
    }
}

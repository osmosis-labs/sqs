use std::{ffi::CString, fmt::Display};

#[repr(C)]
pub struct FFIResult<T> {
    ok: *const T,
    err: *const std::ffi::c_char,
}

impl<T> FFIResult<T> {
    pub fn ok(value: T) -> Self {
        Self {
            ok: Box::into_raw(Box::new(value)),
            err: std::ptr::null(),
        }
    }

    pub fn err<E: Display>(value: E) -> Self {
        let err =
            CString::new(value.to_string()).expect("string must not contain zero internal byte");

        Self {
            ok: std::ptr::null(),
            err: err.into_raw(),
        }
    }
}

impl<T, E: Display> From<Result<T, E>> for FFIResult<T> {
    fn from(value: Result<T, E>) -> Self {
        match value {
            Ok(value) => Self::ok(value),
            Err(value) => Self::err(value),
        }
    }
}

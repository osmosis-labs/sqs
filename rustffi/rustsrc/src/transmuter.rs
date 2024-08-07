use crate::{numbers::FFIDecimal, result::FFIResult};
use transmuter_math::{Division, Timestamp, Uint64};

#[repr(C)]
#[derive(Clone)]
pub struct FFIDivision {
    /// Time where the division is mark as started
    pub started_at: u64,

    /// Time where it is last updated
    pub updated_at: u64,

    /// The latest value that gets updated
    pub latest_value: FFIDecimal,

    /// sum of each updated value * elasped time since last update
    pub integral: FFIDecimal,
}

impl FFIDivision {
    pub fn into_division(self) -> Division {
        Division::unchecked_new(
            Timestamp::from_nanos(self.started_at),
            Timestamp::from_nanos(self.updated_at),
            self.latest_value.into(),
            self.integral.into(),
        )
    }
}

fn ptr_to_option<T: Clone>(ptr: *const T) -> Option<T> {
    if ptr.is_null() {
        None
    } else {
        Some(unsafe { &*ptr }.clone())
    }
}

#[no_mangle]
pub extern "C" fn compressed_moving_average(
    latest_removed_division: *const FFIDivision,
    divisions_ptr: *const FFIDivision,
    divisions_len: usize,
    division_size: u64,
    window_size: u64,
    block_time: u64, // timestamp nanos
) -> FFIResult<FFIDecimal> {
    let latest_removed_division = ptr_to_option(latest_removed_division);
    let divisions = unsafe { std::slice::from_raw_parts(divisions_ptr, divisions_len) }.to_vec();

    let res = transmuter_math::compressed_moving_average(
        latest_removed_division.map(|d| d.into_division()),
        divisions
            .into_iter()
            .map(|d| d.into_division())
            .collect::<Vec<_>>()
            .as_slice(),
        Uint64::from(division_size),
        Uint64::from(window_size),
        Timestamp::from_nanos(block_time),
    );
    match res {
        Ok(decimal) => FFIResult::ok(decimal.into()),
        Err(e) => FFIResult::err(e),
    }
}

// TODO: expose `clean_up_outdated_divisions`

use crate::{
    numbers::FFIDecimal, option::nullable_ptr_to_option, result::FFIResult, slice::FFISlice,
};
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

#[no_mangle]
pub extern "C" fn compressed_moving_average(
    latest_removed_division: *const FFIDivision,
    divisions: FFISlice<FFIDivision>,
    division_size: u64,
    window_size: u64,
    block_time: u64, // timestamp nanos
) -> FFIResult<FFIDecimal> {
    let latest_removed_division = nullable_ptr_to_option(latest_removed_division);
    let divisions = divisions.as_slice().to_vec();

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

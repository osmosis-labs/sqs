use transmuter_math::{Division, Timestamp};

use crate::numbers::FFIDecimal;

#[repr(C)]
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

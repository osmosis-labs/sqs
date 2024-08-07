/// FFI-safe little endian u128 construction
#[repr(C)]
#[derive(Debug)]
pub struct FFIU128([u64; 2]);

impl From<FFIU128> for u128 {
    fn from(value: FFIU128) -> Self {
        value.0[0] as u128 | (value.0[1] as u128) << 64
    }
}

impl From<u128> for FFIU128 {
    fn from(value: u128) -> Self {
        FFIU128([value as u64, (value >> 64) as u64])
    }
}

#[repr(C)]
#[derive(Debug)]
pub struct FFIDecimal(FFIU128);

impl From<FFIDecimal> for transmuter_math::Decimal {
    fn from(value: FFIDecimal) -> Self {
        transmuter_math::Decimal::raw(value.0.into())
    }
}

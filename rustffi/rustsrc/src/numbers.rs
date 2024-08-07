/// FFI-safe little endian u128 construction
#[repr(C)]
#[derive(Clone)]
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
#[derive(Clone)]
pub struct FFIDecimal(FFIU128);

impl From<FFIDecimal> for transmuter_math::Decimal {
    fn from(value: FFIDecimal) -> Self {
        transmuter_math::Decimal::raw(value.0.into())
    }
}

impl From<transmuter_math::Decimal> for FFIDecimal {
    fn from(value: transmuter_math::Decimal) -> Self {
        FFIDecimal(value.atomics().u128().into())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_ffiu128_conversion() {
        // Test small number
        let small: u128 = 42;
        let ffi_small = FFIU128::from(small);
        assert_eq!(u128::from(ffi_small), small);

        // Test large number
        let large: u128 = 0xFFFFFFFFFFFFFFFF_FFFFFFFFFFFFFFFF;
        let ffi_large = FFIU128::from(large);
        assert_eq!(u128::from(ffi_large), large);

        // Test middle range number
        let middle: u128 = 0x0123456789ABCDEF_0123456789ABCDEF;
        let ffi_middle = FFIU128::from(middle);
        assert_eq!(u128::from(ffi_middle), middle);

        // Test zero
        let zero: u128 = 0;
        let ffi_zero = FFIU128::from(zero);
        assert_eq!(u128::from(ffi_zero), zero);

        // Test max u64 + 1
        let over_u64: u128 = 0x0000000000000001_0000000000000000;
        let ffi_over_u64 = FFIU128::from(over_u64);
        assert_eq!(u128::from(ffi_over_u64), over_u64);
    }

    #[test]
    fn test_ffidecimal_conversion() {
        use transmuter_math::Decimal;

        // Test small decimal
        let small = Decimal::raw(42);
        let ffi_small = FFIDecimal::from(small);
        assert_eq!(Decimal::from(ffi_small), small);

        // Test large decimal
        let large = Decimal::raw(340282366920938463463374607431768211455u128); // Max value for Decimal
        let ffi_large = FFIDecimal::from(large);
        assert_eq!(Decimal::from(ffi_large), large);

        // Test middle range decimal
        let middle = Decimal::raw(1000000000000000000u128); // 1.0 in Decimal
        let ffi_middle = FFIDecimal::from(middle);
        assert_eq!(Decimal::from(ffi_middle), middle);

        // Test zero
        let zero = Decimal::zero();
        let ffi_zero = FFIDecimal::from(zero);
        assert_eq!(Decimal::from(ffi_zero), zero);

        // Test fractional decimal
        let fractional = Decimal::raw(1234567890123456789u128); // ~1.234567890123456789
        let ffi_fractional = FFIDecimal::from(fractional);
        assert_eq!(Decimal::from(ffi_fractional), fractional);
    }
}

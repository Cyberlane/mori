pub fn normalize(value: i32) -> i32 { if value < 0 { return 0; } value * 2 }
#[cfg(all(test, not(loom)))]
mod tests {
 fn expected(value: i32) -> i32 { if value < 0 { return 0; } value * 2 }
 #[test]
 fn checks_negative() { assert_eq!(super::normalize(-1), 0); }
}

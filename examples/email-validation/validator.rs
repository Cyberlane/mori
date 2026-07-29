pub fn looks_like_email(value: &str) -> bool {
    let cleaned = value.trim();
    cleaned.contains('@') && cleaned.contains('.')
}

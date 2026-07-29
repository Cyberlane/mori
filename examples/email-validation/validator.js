export function looksLikeEmail(value) {
  const cleaned = value.trim();
  return cleaned.includes("@") && cleaned.includes(".");
}

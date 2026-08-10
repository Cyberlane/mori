function looksLikeEmail(address) {
  const clean = address.trim();
  return clean.includes("@") && clean.includes(".");
}

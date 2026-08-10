namespace EmailValidation;

public static class Validator {
  public static bool LooksLikeEmail(string value) {
    var cleaned = value.Trim();
    return cleaned.Contains("@") && cleaned.Contains(".");
  }
}

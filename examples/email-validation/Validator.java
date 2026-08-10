package emailvalidation;

public final class Validator {
  private Validator() {}

  public static boolean looksLikeEmail(String value) {
    String cleaned = value.trim();
    return cleaned.contains("@") && cleaned.contains(".");
  }
}

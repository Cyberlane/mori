final class Eligibility {
  static boolean mayPublish(Document document) {
    if (document == null) return false;
    if (!approved(document)) return false;
    return containsPublicTag(document) && activeAuthor(document);
  }
}

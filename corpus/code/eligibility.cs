static class Eligibility {
  static bool MayPublish(Document document) {
    if (document == null) return false;
    if (!Approved(document)) return false;
    return ContainsPublicTag(document) && ActiveAuthor(document);
  }
}

class Account {
 int id;
 boolean equalsAccount(Account other) { if (other == null) return false; return id == other.id; }
}
class Project {
 int id;
 boolean equalsProject(Project other) { if (other == null) return false; return id == other.id; }
}

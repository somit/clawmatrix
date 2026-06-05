package database

import (
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	if err := Init("sqlite", dbPath); err != nil {
		t.Fatalf("init db: %v", err)
	}
}

func roleID(t *testing.T, name string) uint {
	t.Helper()
	var r Role
	if err := DB.Where("name = ?", name).First(&r).Error; err != nil {
		t.Fatalf("role %q: %v", name, err)
	}
	return r.ID
}

func mkUser(t *testing.T, name string, systemRole *uint) *User {
	t.Helper()
	u, err := CreateUser(name, "pw", systemRole, nil)
	if err != nil {
		t.Fatalf("create user %q: %v", name, err)
	}
	return u
}

func mkAgent(t *testing.T, id, profile string) {
	t.Helper()
	p := profile
	a := &Agent{ID: id, AgentProfile: &p, Token: "tok-" + id, RegisteredAt: time.Now(), LastHeartbeat: time.Now()}
	if err := DB.Create(a).Error; err != nil {
		t.Fatalf("create agent %q: %v", id, err)
	}
}

// Direct user grant on a profile is visible; an ungranted user sees nothing.
func TestProfileGrant(t *testing.T) {
	setupTestDB(t)
	alice := mkUser(t, "alice", nil)
	bob := mkUser(t, "bob", nil)
	chatter := roleID(t, "chatter")

	if err := SetBinding(PrincipalUser, alice.ID, ResourceProfile, "sales", chatter); err != nil {
		t.Fatal(err)
	}

	if !UserHasProfilePerm(alice.ID, "sales", PermChatWithAgents) {
		t.Error("alice should have chat on sales")
	}
	if UserHasProfilePerm(bob.ID, "sales", PermChatWithAgents) {
		t.Error("bob should NOT have chat on sales")
	}
	// chatter role has no configure perm
	if UserHasProfilePerm(alice.ID, "sales", PermConfigureAgents) {
		t.Error("chatter must not configure")
	}
}

// An agent-level grant gives access to that agent only; the profile is inherited
// additively (a profile grant reaches all its agents).
func TestAgentGrantAndInheritance(t *testing.T) {
	setupTestDB(t)
	alice := mkUser(t, "alice", nil) // agent-level grant only
	carol := mkUser(t, "carol", nil) // profile-level grant
	viewer := roleID(t, "viewer")

	mkAgent(t, "sales-a1", "sales")
	mkAgent(t, "sales-a2", "sales")

	// alice: only the specific agent sales-a1
	if err := SetBinding(PrincipalUser, alice.ID, ResourceAgent, "sales-a1", viewer); err != nil {
		t.Fatal(err)
	}
	// carol: whole profile
	if err := SetBinding(PrincipalUser, carol.ID, ResourceProfile, "sales", viewer); err != nil {
		t.Fatal(err)
	}

	// "two agents in one profile, user sees only one"
	if !UserHasAgentPerm(alice.ID, "sales-a1", PermViewAgents) {
		t.Error("alice should see sales-a1")
	}
	if UserHasAgentPerm(alice.ID, "sales-a2", PermViewAgents) {
		t.Error("alice should NOT see sales-a2")
	}

	// carol inherits both via the profile
	if !UserHasAgentPerm(carol.ID, "sales-a1", PermViewAgents) ||
		!UserHasAgentPerm(carol.ID, "sales-a2", PermViewAgents) {
		t.Error("carol should see both agents via profile")
	}

	// VisibleAgentIDs returns only the directly-granted agent for alice
	ids := VisibleAgentIDs(alice.ID)
	if len(ids) != 1 || ids[0] != "sales-a1" {
		t.Errorf("VisibleAgentIDs(alice) = %v, want [sales-a1]", ids)
	}
}

// Group membership confers the group's grants on members.
func TestGroupGrant(t *testing.T) {
	setupTestDB(t)
	dave := mkUser(t, "dave", nil)
	eve := mkUser(t, "eve", nil) // not a member
	viewer := roleID(t, "viewer")

	g, err := CreateGroup("sales-team", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := AddGroupMember(g.ID, dave.ID); err != nil {
		t.Fatal(err)
	}
	if err := SetBinding(PrincipalGroup, g.ID, ResourceProfile, "sales", viewer); err != nil {
		t.Fatal(err)
	}

	if !UserHasProfilePerm(dave.ID, "sales", PermViewAgents) {
		t.Error("dave should inherit sales view via group")
	}
	if UserHasProfilePerm(eve.ID, "sales", PermViewAgents) {
		t.Error("eve is not a member; should have no access")
	}

	// removing dave revokes access
	if err := RemoveGroupMember(g.ID, dave.ID); err != nil {
		t.Fatal(err)
	}
	if UserHasProfilePerm(dave.ID, "sales", PermViewAgents) {
		t.Error("dave should lose access after removal from group")
	}
}

// VisibleProfiles unions direct and group grants and dedupes.
func TestVisibleProfilesUnion(t *testing.T) {
	setupTestDB(t)
	frank := mkUser(t, "frank", nil)
	viewer := roleID(t, "viewer")

	g, _ := CreateGroup("eng", "")
	AddGroupMember(g.ID, frank.ID)

	SetBinding(PrincipalUser, frank.ID, ResourceProfile, "sales", viewer)
	SetBinding(PrincipalGroup, g.ID, ResourceProfile, "support", viewer)
	SetBinding(PrincipalGroup, g.ID, ResourceProfile, "sales", viewer) // overlap with direct

	profiles, isAdmin := VisibleProfiles(frank.ID)
	if isAdmin {
		t.Fatal("frank is not admin")
	}
	got := map[string]bool{}
	for _, p := range profiles {
		got[p] = true
	}
	if !got["sales"] || !got["support"] {
		t.Errorf("VisibleProfiles = %v, want sales+support", profiles)
	}
	if len(profiles) != 2 {
		t.Errorf("expected deduped 2 profiles, got %v", profiles)
	}
}

// Admin system role bypasses all resource ACLs.
func TestAdminBypass(t *testing.T) {
	setupTestDB(t)
	adminRole, err := AdminRoleID()
	if err != nil {
		t.Fatal(err)
	}
	admin := mkUser(t, "root", &adminRole)

	if !UserHasProfilePerm(admin.ID, "anything", PermConfigureAgents) {
		t.Error("admin should bypass profile ACL")
	}
	if !UserHasAgentPerm(admin.ID, "any-agent", PermChatWithAgents) {
		t.Error("admin should bypass agent ACL")
	}
	if _, isAdmin := VisibleProfiles(admin.ID); !isAdmin {
		t.Error("VisibleProfiles should report admin")
	}
}

// Deleting a user clears their bindings and memberships.
func TestDeleteUserCleansBindings(t *testing.T) {
	setupTestDB(t)
	gina := mkUser(t, "gina", nil)
	viewer := roleID(t, "viewer")
	SetBinding(PrincipalUser, gina.ID, ResourceProfile, "sales", viewer)

	if err := DeleteUser(gina.ID); err != nil {
		t.Fatal(err)
	}
	bindings, _ := ListBindings(ResourceProfile, "sales")
	for _, b := range bindings {
		if b.PrincipalType == PrincipalUser && b.PrincipalID == gina.ID {
			t.Error("gina's binding should be removed on user delete")
		}
	}
}

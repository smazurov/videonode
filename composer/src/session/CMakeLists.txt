# videonode-session — setuid-root PAM verdict helper (Cockpit's
# cockpit-session pattern reduced to a yes/no; the daemon itself runs
# without the shadow group). Reads "username\0password" from stdin —
# never argv or env — runs pam_authenticate + pam_acct_mgmt for service
# "videonode", prints OK / FAIL <reason> and exits. Installed 04755
# root:root by nfpm; /etc/pam.d/videonode gates to the videonode group
# (pam_succeed_if) so the helper only answers for accounts that may log
# in anyway. Plain C: smallest auditable surface for a setuid binary.
# Lives outside bin/ for its scoped .clang-tidy (C, not C++).
if(HAVE_PAM)
    vn_add_executable(videonode-session
        SOURCES videonode_session_main.c
        DEPS    pam_iface)
endif()

#include <errno.h>
#include <security/pam_appl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

enum { kMaxInput = 4096, kMaxUsername = 256, kMaxPamMessages = 32 };

static int fail(const char* token) {
    printf("FAIL %s\n", token);
    return 1;
}

static int read_stdin(char* buf, size_t cap, size_t* len) {
    size_t off = 0;
    while (off < cap) {
        ssize_t n = read(STDIN_FILENO, buf + off, cap - off);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            return -1;
        }
        if (n == 0) {
            *len = off;
            return 0;
        }
        off += (size_t)n;
    }
    return -1;
}

static void free_replies(struct pam_response* replies, int count) {
    for (int i = 0; i < count; i++) {
        free(replies[i].resp);
    }
    free(replies);
}

static int conversation(int num_msg, const struct pam_message** msg, struct pam_response** resp,
                        void* appdata_ptr) {
    const char* password = appdata_ptr;
    if (num_msg <= 0 || num_msg > kMaxPamMessages) {
        return PAM_CONV_ERR;
    }
    struct pam_response* replies = calloc((size_t)num_msg, sizeof(*replies));
    if (replies == NULL) {
        return PAM_BUF_ERR;
    }
    for (int i = 0; i < num_msg; i++) {
        switch (msg[i]->msg_style) {
        case PAM_PROMPT_ECHO_OFF:
            replies[i].resp = strdup(password);
            if (replies[i].resp == NULL) {
                free_replies(replies, i);
                return PAM_BUF_ERR;
            }
            break;
        case PAM_ERROR_MSG:
        case PAM_TEXT_INFO:
            break;
        default:
            free_replies(replies, i);
            return PAM_CONV_ERR;
        }
    }
    *resp = replies;
    return PAM_SUCCESS;
}

static const char* fail_token(int rc) {
    switch (rc) {
    case PAM_USER_UNKNOWN:
        return "unknown_user";
    case PAM_AUTH_ERR:
    case PAM_MAXTRIES:
    case PAM_CRED_INSUFFICIENT:
        return "invalid_password";
    case PAM_ACCT_EXPIRED:
    case PAM_NEW_AUTHTOK_REQD:
        return "account_expired";
    case PAM_PERM_DENIED:
        return "account_locked";
    default:
        return "system_error";
    }
}

int main(void) {
    static char input[kMaxInput];
    size_t len = 0;

    /* argv and the inherited environment are attacker-controlled; drop both. */
    clearenv();
    if (setenv("PATH", "/usr/sbin:/usr/bin:/sbin:/bin", 1) != 0) {
        return fail("system_error");
    }
    /* Normalize real uid to euid so PAM modules see a plain root process. */
    if (geteuid() == 0 && setuid(0) != 0) {
        return fail("system_error");
    }

    if (read_stdin(input, sizeof(input) - 1, &len) != 0) {
        return fail("invalid_input");
    }
    input[len] = '\0';

    const char* username = input;
    const char* sep = memchr(input, '\0', len);
    if (sep == NULL) {
        return fail("invalid_input");
    }
    size_t ulen = (size_t)(sep - input);
    if (ulen == 0 || ulen > kMaxUsername) {
        return fail("invalid_input");
    }
    for (size_t i = 0; i < ulen; i++) {
        unsigned char c = (unsigned char)input[i];
        if (c < 0x20 || c == 0x7f) {
            return fail("invalid_input");
        }
    }
    const char* password = sep + 1;
    size_t plen = len - ulen - 1;
    if (memchr(password, '\0', plen) != NULL) {
        return fail("invalid_input");
    }

    struct pam_conv conv = {conversation, (void*)password};
    pam_handle_t* pamh = NULL;
    int rc = pam_start("videonode", username, &conv, &pamh);
    if (rc != PAM_SUCCESS) {
        return fail("system_error");
    }
    rc = pam_authenticate(pamh, PAM_SILENT | PAM_DISALLOW_NULL_AUTHTOK);
    if (rc == PAM_SUCCESS) {
        rc = pam_acct_mgmt(pamh, PAM_SILENT | PAM_DISALLOW_NULL_AUTHTOK);
    }
    pam_end(pamh, rc);
    explicit_bzero(input, sizeof(input));

    if (rc != PAM_SUCCESS) {
        return fail(fail_token(rc));
    }
    printf("OK\n");
    return 0;
}

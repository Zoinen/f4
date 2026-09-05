#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>
#include <unistd.h>
#include <errno.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/wait.h>

#include "../include/vtui.h"

static char g_last_error[256] = "";

static void set_last_error(const char *err) {
    if (err) {
        strncpy(g_last_error, err, sizeof(g_last_error) - 1);
        g_last_error[sizeof(g_last_error) - 1] = '\0';
    }
}

const char *vtui_last_error(void) {
    return g_last_error;
}

struct vtui_session {
    pid_t pid;
    int fd;
    FILE *stream_in;
    FILE *stream_out;
};

vtui_session *vtui_open(const char *config_json) {
    signal(SIGPIPE, SIG_IGN);

    int sv[2];
    if (socketpair(AF_UNIX, SOCK_STREAM, 0, sv) < 0) {
        set_last_error("failed to create socketpair");
        return NULL;
    }

    const char *host_bin = getenv("VTUI_HOST_BIN");
    if (!host_bin) host_bin = "vtui-host";

    const char *backend = getenv("VTUI_BACKEND");
    if (!backend) backend = "ansi";

    pid_t pid = fork();
    if (pid < 0) {
        close(sv[0]);
        close(sv[1]);
        set_last_error("fork failed");
        return NULL;
    }

    if (pid == 0) {
        // Child: run vtui-host with protocol fd 3
        close(sv[0]);
        if (sv[1] != 3) {
            dup2(sv[1], 3);
            close(sv[1]);
        }
        char backend_arg[64];
        snprintf(backend_arg, sizeof(backend_arg), "--backend=%s", backend);

        if (getenv("VTUI_HOST_BIN")) {
            execl(host_bin, host_bin, "--protocol-fd=3", backend_arg, (char *)NULL);
        }
        execl("./vtui-host", "./vtui-host", "--protocol-fd=3", backend_arg, (char *)NULL);
        execl("../vtui-host", "../vtui-host", "--protocol-fd=3", backend_arg, (char *)NULL);
        execl("../../vtui-host", "../../vtui-host", "--protocol-fd=3", backend_arg, (char *)NULL);
        execl("../../../vtui-host", "../../../vtui-host", "--protocol-fd=3", backend_arg, (char *)NULL);
        execl("../../../cmd/vtui-host/vtui-host", "vtui-host", "--protocol-fd=3", backend_arg, (char *)NULL);
        execlp(host_bin, host_bin, "--protocol-fd=3", backend_arg, (char *)NULL);
        fprintf(stderr, "vtui: failed to execute vtui-host (binary not found in PATH or build directory)\n");
        _exit(127);
    }

    // Parent
    close(sv[1]);
    vtui_session *s = (vtui_session *)malloc(sizeof(vtui_session));
    s->pid = pid;
    s->fd = sv[0];
    s->stream_in = fdopen(dup(sv[0]), "r");
    s->stream_out = fdopen(sv[0], "w");

    // Perform handshake
    vtui_send(s, "{\"op\":\"hello\",\"version\":1}\n", 28);
    char buf[1024];
    size_t out_len = 0;
    if (vtui_recv(s, buf, sizeof(buf), &out_len) != 0) {
        vtui_close(s);
        set_last_error("handshake failed with vtui-host");
        return NULL;
    }

    return s;
}

int vtui_send(vtui_session *s, const char *line, size_t len) {
    if (!s || s->fd < 0) return -1;
    ssize_t written = write(s->fd, line, len);
    if (written < 0) return -1;
    if (len == 0 || line[len - 1] != '\n') {
        if (write(s->fd, "\n", 1) < 0) return -1;
    }
    return 0;
}

int vtui_event_fd(vtui_session *s) {
    if (!s) return -1;
    return s->fd;
}

int vtui_recv(vtui_session *s, char *buf, size_t cap, size_t *out_len) {
    if (!s || !s->stream_in) return -1;
    while (fgets(buf, (int)cap, s->stream_in) == NULL) {
        if (errno == EINTR) {
            continue;
        }
        return -1;
    }
    if (out_len) {
        *out_len = strlen(buf);
    }
    return 0;
}

void vtui_close(vtui_session *s) {
    if (!s) return;
    vtui_send(s, "{\"op\":\"quit\"}\n", 14);
    if (s->fd >= 0) {
        shutdown(s->fd, SHUT_WR);
    }
    if (s->pid > 0) {
        int status = 0;
        waitpid(s->pid, &status, 0);
    }
    if (s->stream_in) fclose(s->stream_in);
    if (s->stream_out) fclose(s->stream_out);
    if (s->fd >= 0) close(s->fd);
    free(s);
}

/* --- Immediate-Mode Facade --- */

struct vtui_ui {
    vtui_session *session;
    char dialog_title[128];
    int dialog_width;
    char current_children[4096];
    char last_cmd_src[64];
    int mounted;
    char *last_edit_buf;
    size_t last_edit_cap;
};

void vtui_dialog(vtui_ui *u, const char *title, int w) {
    if (!u) return;
    strncpy(u->dialog_title, title ? title : "", sizeof(u->dialog_title) - 1);
    u->dialog_width = w > 0 ? w : 40;
    u->current_children[0] = '\0';
}

void vtui_edit(vtui_ui *u, const char *label, char *buf, size_t buf_cap) {
    if (!u) return;
    u->last_edit_buf = buf;
    u->last_edit_cap = buf_cap;
    char entry[512];
    snprintf(entry, sizeof(entry),
        "%s{\"type\":\"Group\",\"layout\":{\"type\":\"Form\",\"spacing\":1},\"children\":["
        "{\"type\":\"Label\",\"props\":{\"text\":\"%s\"}},"
        "{\"type\":\"Edit\",\"id\":\"nameEdit\",\"props\":{\"text\":\"%s\"}}]}",
        (u->current_children[0] != '\0') ? "," : "",
        label ? label : "",
        buf ? buf : "");
    strncat(u->current_children, entry, sizeof(u->current_children) - strlen(u->current_children) - 1);
}

int vtui_button(vtui_ui *u, const char *text) {
    if (!u) return 0;
    char entry[256];
    snprintf(entry, sizeof(entry),
        "%s{\"type\":\"Button\",\"id\":\"okBtn\",\"props\":{\"text\":\"%s\",\"command\":1000}}",
        (u->current_children[0] != '\0') ? "," : "",
        text ? text : "&Ok");
    strncat(u->current_children, entry, sizeof(u->current_children) - strlen(u->current_children) - 1);
    if (strcmp(u->last_cmd_src, "okBtn") == 0) {
        u->last_cmd_src[0] = '\0';
        return 1;
    }
    return 0;
}

int vtui_checkbox(vtui_ui *u, const char *text, int default_state) {
    if (!u) return 0;
    char entry[256];
    snprintf(entry, sizeof(entry),
        "%s{\"type\":\"Checkbox\",\"props\":{\"text\":\"%s\",\"state\":%d}}",
        (u->current_children[0] != '\0') ? "," : "",
        text ? text : "",
        default_state ? 1 : 0);
    strncat(u->current_children, entry, sizeof(u->current_children) - strlen(u->current_children) - 1);
    return default_state;
}

static void escape_json_str(const char *in, char *out, size_t out_cap) {
    if (!in || !out || out_cap == 0) return;
    size_t j = 0;
    for (size_t i = 0; in[i] != '\0' && j + 2 < out_cap; i++) {
        if (in[i] == '"') {
            out[j++] = '\\'; out[j++] = '"';
        } else if (in[i] == '\\') {
            out[j++] = '\\'; out[j++] = '\\';
        } else if (in[i] == '\n') {
            out[j++] = '\\'; out[j++] = 'n';
        } else if (in[i] == '\r') {
            out[j++] = '\\'; out[j++] = 'r';
        } else if (in[i] == '\t') {
            out[j++] = '\\'; out[j++] = 't';
        } else {
            out[j++] = in[i];
        }
    }
    out[j] = '\0';
}

void vtui_message(vtui_ui *u, const char *title, const char *text) {
    if (!u || !u->session) return;
    char esc_title[128];
    char esc_text[1024];
    escape_json_str(title ? title : " Message ", esc_title, sizeof(esc_title));
    escape_json_str(text ? text : "", esc_text, sizeof(esc_text));
    char msg[2048];
    snprintf(msg, sizeof(msg),
        "{\"op\":\"message\",\"title\":\"%s\",\"text\":\"%s\",\"buttons\":[\"&Ok\"]}\n",
        esc_title, esc_text);
    vtui_send(u->session, msg, strlen(msg));
}

void vtui_end(vtui_ui *u) {
    if (!u || !u->session) return;
    if (!u->mounted) {
        char mount[8192];
        snprintf(mount, sizeof(mount),
            "{\"op\":\"mount\",\"frameId\":\"mainDlg\",\"tree\":{"
            "\"type\":\"Dialog\",\"id\":\"mainDlg\",\"props\":{\"title\":\"%s\",\"autoSize\":true,\"center\":true},"
            "\"layout\":{\"type\":\"VBox\",\"spacing\":1,\"margins\":[1,2,1,2]},"
            "\"children\":[%s]}}\n",
            u->dialog_title, u->current_children);
        vtui_send(u->session, mount, strlen(mount));
        u->mounted = 1;
    }
    u->last_cmd_src[0] = '\0';
}

int vtui_run(vtui_ui_func ui_fn) {
    if (!ui_fn) return 1;

    vtui_session *s = vtui_open("{\"backend\":\"ansi\"}");
    if (!s) {
        fprintf(stderr, "vtui_open failed: %s\n", vtui_last_error());
        return 1;
    }

    vtui_ui u;
    memset(&u, 0, sizeof(u));
    u.session = s;

    ui_fn(&u);

    char buf[4096];
    size_t out_len = 0;
    while (vtui_recv(s, buf, sizeof(buf) - 1, &out_len) == 0) {
        if (out_len > 0) {
            buf[out_len] = '\0';
            if (strstr(buf, "\"op\":\"closed\"") != NULL) {
                if (strstr(buf, "\"frameId\":\"mainDlg\"") != NULL) {
                    break;
                }
            }
            if (strstr(buf, "\"op\":\"changed\"") != NULL) {
                char *val = strstr(buf, "\"value\":\"");
                if (val && u.last_edit_buf && u.last_edit_cap > 0) {
                    val += 9;
                    char *end = strchr(val, '"');
                    if (end) {
                        size_t len = end - val;
                        if (len >= u.last_edit_cap) len = u.last_edit_cap - 1;
                        strncpy(u.last_edit_buf, val, len);
                        u.last_edit_buf[len] = '\0';
                    }
                }
            }
            if (strstr(buf, "\"op\":\"command\"") != NULL) {
                char *src = strstr(buf, "\"srcId\":\"");
                if (src) {
                    src += 9;
                    char *end = strchr(src, '"');
                    if (end) {
                        size_t len = end - src;
                        if (len >= sizeof(u.last_cmd_src)) len = sizeof(u.last_cmd_src) - 1;
                        strncpy(u.last_cmd_src, src, len);
                        u.last_cmd_src[len] = '\0';
                    }
                }
            }
            ui_fn(&u);
        }
    }

    vtui_close(s);
    return 0;
}

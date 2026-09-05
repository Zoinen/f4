#include <lua.h>
#include <lauxlib.h>
#include <lualib.h>

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/wait.h>
#include <signal.h>

typedef struct {
    int fd;
    pid_t pid;
    FILE *stream_in;
    FILE *stream_out;
} l_vtui_session;

static int l_vtui_open(lua_State *L) {
    const char *host_bin = luaL_optstring(L, 1, "vtui-host");
    const char *backend = luaL_optstring(L, 2, "ansi");

    signal(SIGPIPE, SIG_IGN);

    int sv[2];
    if (socketpair(AF_UNIX, SOCK_STREAM, 0, sv) < 0) {
        return luaL_error(L, "vtui: socketpair failed: %s", strerror(errno));
    }

    pid_t pid = fork();
    if (pid < 0) {
        close(sv[0]);
        close(sv[1]);
        return luaL_error(L, "vtui: fork failed: %s", strerror(errno));
    }

    if (pid == 0) {
        close(sv[0]);
        if (sv[1] != 3) {
            dup2(sv[1], 3);
            close(sv[1]);
        }
        char backend_arg[64];
        snprintf(backend_arg, sizeof(backend_arg), "--backend=%s", backend);
        execlp(host_bin, host_bin, "--protocol-fd=3", backend_arg, (char *)NULL);
        _exit(127);
    }

    close(sv[1]);

    l_vtui_session *s = (l_vtui_session *)lua_newuserdata(L, sizeof(l_vtui_session));
    s->fd = sv[0];
    s->pid = pid;
    s->stream_in = fdopen(dup(sv[0]), "r");
    s->stream_out = fdopen(sv[0], "w");

    luaL_getmetatable(L, "vtui_session_meta");
    lua_setmetatable(L, -2);
    return 1;
}

static int l_vtui_send(lua_State *L) {
    l_vtui_session *s = (l_vtui_session *)luaL_checkudata(L, 1, "vtui_session_meta");
    size_t len = 0;
    const char *line = luaL_checklstring(L, 2, &len);
    if (!s || s->fd < 0) return 0;

    ssize_t w = write(s->fd, line, len);
    if (w < 0) {
        return luaL_error(L, "vtui: write error: %s", strerror(errno));
    }
    if (len == 0 || line[len - 1] != '\n') {
        if (write(s->fd, "\n", 1) < 0) {
            return luaL_error(L, "vtui: write error: %s", strerror(errno));
        }
    }
    lua_pushboolean(L, 1);
    return 1;
}

static int l_vtui_recv(lua_State *L) {
    l_vtui_session *s = (l_vtui_session *)luaL_checkudata(L, 1, "vtui_session_meta");
    if (!s || !s->stream_in) {
        lua_pushnil(L);
        return 1;
    }

    char buf[8192];
    while (fgets(buf, sizeof(buf), s->stream_in) == NULL) {
        if (errno == EINTR) continue;
        lua_pushnil(L);
        return 1;
    }
    lua_pushstring(L, buf);
    return 1;
}

static int l_vtui_close(lua_State *L) {
    l_vtui_session *s = (l_vtui_session *)luaL_checkudata(L, 1, "vtui_session_meta");
    if (!s) return 0;
    if (s->fd >= 0) {
        write(s->fd, "{\"op\":\"quit\"}\n", 14);
        shutdown(s->fd, SHUT_WR);
    }
    if (s->pid > 0) {
        int status = 0;
        waitpid(s->pid, &status, 0);
        s->pid = 0;
    }
    if (s->stream_in) { fclose(s->stream_in); s->stream_in = NULL; }
    if (s->stream_out) { fclose(s->stream_out); s->stream_out = NULL; }
    if (s->fd >= 0) { close(s->fd); s->fd = -1; }
    return 0;
}

static const struct luaL_Reg vtui_funcs[] = {
    {"open", l_vtui_open},
    {"send", l_vtui_send},
    {"recv", l_vtui_recv},
    {"close", l_vtui_close},
    {NULL, NULL}
};

static const struct luaL_Reg vtui_methods[] = {
    {"send", l_vtui_send},
    {"recv", l_vtui_recv},
    {"close", l_vtui_close},
    {"__gc", l_vtui_close},
    {NULL, NULL}
};

int luaopen_vtui_lua(lua_State *L) {
    luaL_newmetatable(L, "vtui_session_meta");
    lua_pushvalue(L, -1);
    lua_setfield(L, -2, "__index");
#if LUA_VERSION_NUM >= 502
    luaL_setfuncs(L, vtui_methods, 0);
#else
    luaL_register(L, NULL, vtui_methods);
#endif
    lua_pop(L, 1);

#if LUA_VERSION_NUM >= 502
    luaL_newlib(L, vtui_funcs);
#else
    luaL_register(L, "vtui_lua", vtui_funcs);
#endif
    return 1;
}

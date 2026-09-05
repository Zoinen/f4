#ifndef VTUI_H
#define VTUI_H

#include <stddef.h>
#include <stdint.h>
#include "vtui_constants.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct vtui_session vtui_session;

/* Low-level C ABI (Section 9.2) */
vtui_session *vtui_open(const char *config_json);
int           vtui_send(vtui_session *s, const char *line, size_t len);
int           vtui_event_fd(vtui_session *s);
int           vtui_recv(vtui_session *s, char *buf, size_t cap, size_t *out_len);
void          vtui_close(vtui_session *s);
const char   *vtui_last_error(void);

/* Idiomatic C Immediate-Mode Facade (Section 10.2) */
typedef struct vtui_ui vtui_ui;
typedef void (*vtui_ui_func)(vtui_ui *u);

int  vtui_run(vtui_ui_func ui_fn);
void vtui_dialog(vtui_ui *u, const char *title, int w);
void vtui_edit(vtui_ui *u, const char *label, char *buf, size_t buf_cap);
int  vtui_button(vtui_ui *u, const char *text);
int  vtui_checkbox(vtui_ui *u, const char *text, int default_state);
void vtui_message(vtui_ui *u, const char *title, const char *text);
void vtui_end(vtui_ui *u);

#ifdef __cplusplus
}
#endif

#endif /* VTUI_H */

#ifndef VESTA_OPAQUE_H
#define VESTA_OPAQUE_H

#include <stddef.h>
#include <stdint.h>

typedef struct {
    void *state;
    uint8_t message[98];
} VestaOpaqueStart;

typedef struct {
    uint8_t message[64];
    uint8_t session_key[64];
    uint8_t export_key[64];
} VestaOpaqueFinish;

int32_t vesta_opaque_begin(const uint8_t *password, size_t password_len, VestaOpaqueStart *output);
int32_t vesta_opaque_finish(void **state, const uint8_t *response, size_t response_len, VestaOpaqueFinish *output);
void vesta_opaque_free(void **state);

#endif

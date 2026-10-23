/* Isolated-test-only fault injector. Never install in a production container.
 * Delay the real client's first SHOW CREATE DATABASE send, which mysqldump
 * performs after UNLOCK TABLES and before its Setting-savepoint message.
 * The FTWRL fixture pauses before that send to admit a real competing query.
 * No MySQL source, credentials, or business data are changed or logged.
 * Freestanding declarations allow cross-building this tiny Linux ELF on macOS.
 */
typedef unsigned long size_t;
typedef long ssize_t;
struct timespec { long tv_sec; long tv_nsec; };
extern void *dlsym(void *, const char *);
extern void *memmem(const void *, size_t, const void *, size_t);
extern int nanosleep(const struct timespec *, struct timespec *);
extern int open(const char *, int, ...);
extern int close(int);
extern char *getenv(const char *);
extern size_t strlen(const char *);

static void delay(const void *buffer, size_t length) {
    static int completed;
    int before_ftwrl = getenv("NEWAPI_PITR_TEST_FTWRL_DELAY") != 0;
    const char *text = before_ftwrl ? "FLUSH TABLES WITH READ LOCK" : "SHOW CREATE DATABASE";
    if (!completed && memmem(buffer, length, text, strlen(text))) {
        completed = 1;
        const char *path = getenv("NEWAPI_PITR_DELAY_MARKER");
        if (path) {
            int fd = open(path, 577, 0600); /* WRONLY|CREAT|TRUNC */
            if (fd >= 0) close(fd);
        }
        struct timespec remaining = {before_ftwrl ? 2 : 7, 0};
        while (nanosleep(&remaining, &remaining)) {}
    }
}

ssize_t send(int fd, const void *buffer, size_t length, int flags) {
    ssize_t (*original)(int,const void *,size_t,int) = dlsym((void *)-1L, "send");
    delay(buffer, length);
    return original(fd, buffer, length, flags);
}

ssize_t write(int fd, const void *buffer, size_t length) {
    ssize_t (*original)(int,const void *,size_t) = dlsym((void *)-1L, "write");
    delay(buffer, length);
    return original(fd, buffer, length);
}

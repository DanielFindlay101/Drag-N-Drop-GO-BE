
CREATE TABLE IF NOT EXISTS users (
    id              CHAR(36)        NOT NULL PRIMARY KEY,
    email           VARCHAR(255)    NOT NULL UNIQUE,
    passwordHash    VARCHAR(255)    NOT NULL,
    createdAt       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP
);


CREATE TABLE IF NOT EXISTS tasks (
    id          CHAR(36)     NOT NULL PRIMARY KEY,
    userId      CHAR(36)     NOT NULL,
    title       VARCHAR(255) NOT NULL,
    description VARCHAR(500) NOT NULL,
    tag         VARCHAR(50)  NOT NULL,
    colour      VARCHAR(20)  NOT NULL,
    completed   BOOLEAN      NOT NULL DEFAULT FALSE,
    createdAt   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updatedAt   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (userId) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    tokenHash   CHAR(64)    NOT NULL PRIMARY KEY,   -- sha256, hex encoded
    userId      CHAR(36)    NOT NULL,
    expiresAt   DATETIME    NOT NULL,
    revokedAt   DATETIME    NULL,                   -- set when rotated
    createdAt   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (userId) REFERENCES users(id) ON DELETE CASCADE
);

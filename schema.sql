CREATE TABLE IF NOT EXISTS users (
    id              INTEGER PRIMARY KEY,
    email           TEXT    NOT NULL UNIQUE,
    password_hash   TEXT    NOT NULL DEFAULT '',
    username        TEXT    NOT NULL DEFAULT '',
    error_count     INTEGER NOT NULL DEFAULT 0,
    last_error_at   DATETIME,
    locked_until    DATETIME,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS equipment (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    equipment_no    TEXT    NOT NULL UNIQUE,
    user_id         INTEGER,
    name            TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'ACTIVE',
    total_capacity  TEXT    NOT NULL DEFAULT '0',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS files (
    id              INTEGER PRIMARY KEY,
    user_id         INTEGER NOT NULL,
    directory_id    INTEGER NOT NULL DEFAULT 0,
    file_name       TEXT    NOT NULL,
    inner_name      TEXT    NOT NULL DEFAULT '',
    md5             TEXT    NOT NULL DEFAULT '',
    size            INTEGER NOT NULL DEFAULT 0,
    is_folder       TEXT    NOT NULL DEFAULT 'N',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_files_user_dir ON files(user_id, directory_id);

CREATE TABLE IF NOT EXISTS schedule_groups (
    task_list_id    TEXT    PRIMARY KEY,
    user_id         INTEGER NOT NULL,
    title           TEXT    NOT NULL DEFAULT '',
    last_modified   INTEGER NOT NULL DEFAULT 0,
    create_time     INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS schedule_tasks (
    task_id             TEXT    PRIMARY KEY,
    user_id             INTEGER NOT NULL,
    task_list_id        TEXT    NOT NULL DEFAULT '',
    title               TEXT    NOT NULL DEFAULT '',
    detail              TEXT    NOT NULL DEFAULT '',
    last_modified       INTEGER NOT NULL DEFAULT 0,
    recurrence          TEXT    NOT NULL DEFAULT '',
    is_reminder_on      TEXT    NOT NULL DEFAULT 'N',
    status              TEXT    NOT NULL DEFAULT 'needsAction',
    importance          TEXT    NOT NULL DEFAULT '',
    due_time            INTEGER NOT NULL DEFAULT 0,
    completed_time      INTEGER NOT NULL DEFAULT 0,
    links               TEXT    NOT NULL DEFAULT '',
    sort                INTEGER NOT NULL DEFAULT 0,
    sort_completed      INTEGER NOT NULL DEFAULT 0,
    planer_sort         INTEGER NOT NULL DEFAULT 0,
    sort_time           INTEGER NOT NULL DEFAULT 0,
    planer_sort_time    INTEGER NOT NULL DEFAULT 0,
    all_sort            INTEGER NOT NULL DEFAULT 0,
    all_sort_completed  INTEGER NOT NULL DEFAULT 0,
    all_sort_time       INTEGER NOT NULL DEFAULT 0,
    recurrence_id       TEXT    NOT NULL DEFAULT '',
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS summaries (
    id                          INTEGER PRIMARY KEY,
    user_id                     INTEGER NOT NULL,
    unique_identifier           TEXT    NOT NULL DEFAULT '',
    name                        TEXT    NOT NULL DEFAULT '',
    description                 TEXT    NOT NULL DEFAULT '',
    file_id                     INTEGER NOT NULL DEFAULT 0,
    parent_unique_identifier    TEXT    NOT NULL DEFAULT '',
    content                     TEXT    NOT NULL DEFAULT '',
    data_source                 TEXT    NOT NULL DEFAULT '',
    source_path                 TEXT    NOT NULL DEFAULT '',
    source_type                 INTEGER NOT NULL DEFAULT 0,
    tags                        TEXT    NOT NULL DEFAULT '',
    md5_hash                    TEXT    NOT NULL DEFAULT '',
    metadata                    TEXT    NOT NULL DEFAULT '',
    comment_str                 TEXT    NOT NULL DEFAULT '',
    comment_handwrite_name      TEXT    NOT NULL DEFAULT '',
    handwrite_inner_name        TEXT    NOT NULL DEFAULT '',
    handwrite_md5               TEXT    NOT NULL DEFAULT '',
    is_summary_group            TEXT    NOT NULL DEFAULT 'N',
    author                      TEXT    NOT NULL DEFAULT '',
    creation_time               INTEGER NOT NULL DEFAULT 0,
    last_modified_time          INTEGER NOT NULL DEFAULT 0,
    created_at                  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at                  DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_summaries_user ON summaries(user_id, is_summary_group);

CREATE TABLE IF NOT EXISTS chunk_uploads (
    upload_id       TEXT    NOT NULL,
    part_number     INTEGER NOT NULL,
    total_chunks    INTEGER NOT NULL,
    chunk_md5       TEXT    NOT NULL DEFAULT '',
    path            TEXT    NOT NULL DEFAULT '',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (upload_id, part_number)
);

CREATE TABLE IF NOT EXISTS login_challenges (
    account         TEXT    NOT NULL,
    timestamp       INTEGER NOT NULL,
    random_code     TEXT    NOT NULL,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account, timestamp)
);

CREATE TABLE IF NOT EXISTS sync_locks (
    user_id         INTEGER PRIMARY KEY,
    equipment_no    TEXT    NOT NULL,
    expires_at      DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS server_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS auth_tokens (
    key             TEXT    PRIMARY KEY,
    token           TEXT    NOT NULL,
    user_id         INTEGER NOT NULL,
    equipment_no    TEXT    NOT NULL DEFAULT '',
    expires_at      DATETIME NOT NULL,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

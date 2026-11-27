ALTER TABLE recommendations.stand_accounts
    ADD COLUMN IF NOT EXISTS role text NOT NULL DEFAULT 'unverified'
    CHECK (role IN ('unverified', 'verified', 'admin'));

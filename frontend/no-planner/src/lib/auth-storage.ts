export type StoredAuthSession = {
  accessToken: string;
  user: {
    subject: string;
    email: string;
    name: string;
    pictureUrl: string;
  };
};

const storageKey = "noplanner.auth.session";

export type AuthStorage = {
  load(): StoredAuthSession | null;
  save(session: StoredAuthSession): void;
  clear(): void;
};

export function createAuthStorage(storage: Storage): AuthStorage {
  return {
    load() {
      const serialized = storage.getItem(storageKey);
      if (serialized === null) return null;

      try {
        const session: unknown = JSON.parse(serialized);
        return isStoredAuthSession(session) ? session : null;
      } catch {
        return null;
      }
    },
    save(session) {
      storage.setItem(storageKey, JSON.stringify(session));
    },
    clear() {
      storage.removeItem(storageKey);
    },
  };
}

function isStoredAuthSession(value: unknown): value is StoredAuthSession {
  if (typeof value !== "object" || value === null) return false;
  const session = value as Partial<StoredAuthSession>;
  if (typeof session.accessToken !== "string" || session.accessToken.length === 0) return false;
  if (typeof session.user !== "object" || session.user === null) return false;
  return (
    typeof session.user.subject === "string" &&
    session.user.subject.length > 0 &&
    typeof session.user.email === "string" &&
    typeof session.user.name === "string" &&
    typeof session.user.pictureUrl === "string"
  );
}

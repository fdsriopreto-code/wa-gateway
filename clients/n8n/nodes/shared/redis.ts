import type { IDataObject } from 'n8n-workflow';
import Redis from 'ioredis';
import type { RedisOptions } from 'ioredis';

/**
 * Monta um cliente ioredis a partir da credencial `redis` nativa do n8n
 * (campos host/port/database/user/password/ssl). Curto-vivo: use e feche.
 */
export function redisFromCreds(creds: IDataObject): Redis {
	const opts: RedisOptions = {
		host: (creds.host as string) || 'localhost',
		port: Number(creds.port) || 6379,
		db: Number(creds.database) || 0,
		connectTimeout: 8000,
		maxRetriesPerRequest: 1,
		retryStrategy: () => null,
	};
	if (creds.password) opts.password = creds.password as string;
	if (creds.user) opts.username = creds.user as string;
	if (creds.ssl === true) opts.tls = {};
	return new Redis(opts);
}

/**
 * Coleta atômica da fila: só devolve as mensagens se o marcador ainda for
 * desta execução (ARGV[1]). Se outra mensagem chegou depois, o marcador
 * mudou e o script devolve nil -> esta execução foi "superada".
 *   KEYS[1] = lista da fila   KEYS[2] = marcador   ARGV[1] = token
 */
export const COLLECT_LUA = `
local m = redis.call('GET', KEYS[2])
if not m or m ~= ARGV[1] then return nil end
local items = redis.call('LRANGE', KEYS[1], 0, -1)
redis.call('DEL', KEYS[1], KEYS[2])
return items
`;

/** prefixo padronizado das chaves: wa:[ns:]<kind>:<key> */
export function redisKey(ns: string, kind: string, key: string): string {
	return `wa:${ns ? ns + ':' : ''}${kind}:${key}`;
}

<?php

namespace BreeferCP;

class ApiException extends \RuntimeException
{
    private string $body;

    public function __construct(string $message, int $code = 0, string $body = '')
    {
        parent::__construct($message, $code);
        $this->body = $body;
    }

    public function getBody(): string
    {
        return $this->body;
    }
}

/**
 * Minimal BreeferCP API client. Uses cURL with certificate verification on;
 * TLS verification is never disabled.
 */
class Client
{
    private string $base;
    private string $token;

    public function __construct(string $base, string $token)
    {
        if ($token === '') {
            throw new ApiException('No API token configured on the WHMCS server entry (Access Hash).');
        }
        $this->base = rtrim($base, '/') . '/api/v1';
        $this->token = $token;
    }

    public function get(string $path): array
    {
        return $this->request('GET', $path);
    }

    public function post(string $path, array $body = []): array
    {
        return $this->request('POST', $path, $body);
    }

    private function request(string $method, string $path, ?array $body = null): array
    {
        $ch = curl_init($this->base . $path);
        $headers = [
            'Authorization: Bearer ' . $this->token,
            'Accept: application/json',
            'Content-Type: application/json',
        ];
        curl_setopt_array($ch, [
            CURLOPT_CUSTOMREQUEST => $method,
            CURLOPT_HTTPHEADER => $headers,
            CURLOPT_RETURNTRANSFER => true,
            CURLOPT_TIMEOUT => 60,
            CURLOPT_CONNECTTIMEOUT => 10,
            CURLOPT_SSL_VERIFYPEER => true,
            CURLOPT_SSL_VERIFYHOST => 2,
            CURLOPT_FOLLOWLOCATION => false,
        ]);
        if ($body !== null) {
            curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($body, JSON_THROW_ON_ERROR));
        }
        $raw = curl_exec($ch);
        if ($raw === false) {
            $err = curl_error($ch);
            curl_close($ch);
            throw new ApiException('Connection to BreeferCP failed: ' . $err);
        }
        $status = (int) curl_getinfo($ch, CURLINFO_RESPONSE_CODE);
        curl_close($ch);

        $data = $raw === '' ? [] : json_decode($raw, true);
        if ($status >= 400) {
            $msg = is_array($data) && isset($data['error']['message']) ? $data['error']['message'] : "HTTP $status";
            throw new ApiException($msg, $status, (string) $raw);
        }
        return is_array($data) ? $data : [];
    }
}

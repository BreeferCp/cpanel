<?php
/**
 * BreeferCP single sign-on for phpMyAdmin.
 *
 * Install at /usr/share/phpmyadmin/breefercp-signon.php and configure the
 * server in config.inc.php with:
 *
 *   $cfg['Servers'][1]['auth_type']     = 'signon';
 *   $cfg['Servers'][1]['SignonSession'] = 'BreeferPMA';
 *   $cfg['Servers'][1]['SignonURL']     = 'breefercp-signon.php';
 *   $cfg['Servers'][1]['host']          = 'localhost';
 *
 * The panel hands the browser a one-time token. This script redeems it with
 * the panel over loopback for temporary MariaDB credentials that can reach
 * only that account's databases, stores them in the signon session and sends
 * the browser into phpMyAdmin. The token is useless after one use or two
 * minutes, and the temporary database user is dropped by the panel later.
 */

declare(strict_types=1);

const PANEL = 'http://127.0.0.1:2087/api/v1/internal/pma/redeem';

function fail(string $msg): never
{
    http_response_code(403);
    header('Content-Type: text/plain; charset=utf-8');
    echo $msg, "\n";
    exit;
}

$token = $_GET['token'] ?? '';
if (!is_string($token) || !preg_match('/^[A-Za-z0-9_-]{20,100}$/', $token)) {
    fail('Open phpMyAdmin from your BreeferCP control panel.');
}

$ch = curl_init(PANEL);
curl_setopt_array($ch, [
    CURLOPT_POST => true,
    CURLOPT_POSTFIELDS => json_encode(['token' => $token], JSON_THROW_ON_ERROR),
    CURLOPT_HTTPHEADER => ['Content-Type: application/json', 'Accept: application/json'],
    CURLOPT_RETURNTRANSFER => true,
    CURLOPT_TIMEOUT => 10,
]);
$raw = curl_exec($ch);
$status = (int) curl_getinfo($ch, CURLINFO_RESPONSE_CODE);
curl_close($ch);

$data = is_string($raw) ? json_decode($raw, true) : null;
if ($status !== 200 || !is_array($data) || empty($data['user'])) {
    fail('This phpMyAdmin link has expired. Open it again from the control panel.');
}

// Strict session cookie, separate from phpMyAdmin's own.
session_set_cookie_params(['httponly' => true, 'secure' => true, 'samesite' => 'Strict']);
session_name('BreeferPMA');
session_start();
session_regenerate_id(true);
$_SESSION['PMA_single_signon_user'] = $data['user'];
$_SESSION['PMA_single_signon_password'] = $data['password'];
$_SESSION['PMA_single_signon_host'] = $data['host'] ?? 'localhost';
session_write_close();

header('Location: index.php?server=1');
exit;

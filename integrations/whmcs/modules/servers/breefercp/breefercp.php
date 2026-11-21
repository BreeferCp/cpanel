<?php
/**
 * BreeferCP provisioning module for WHMCS.
 *
 * Install: copy modules/servers/breefercp to <whmcs>/modules/servers/breefercp.
 * In WHMCS add a server of type "BreeferCP":
 *   - Hostname:    the panel hostname (e.g. panel.example.com)
 *   - Access Hash: a BreeferCP API token with the "billing" scope
 *   - Port:        2087 (the panel port); tick "Secure" for HTTPS
 * In the product, set "Package ID" to the BreeferCP package that the product maps to.
 *
 * Every call is idempotent on the WHMCS service id (sent as external_ref), so
 * WHMCS retries never create duplicate accounts.
 */

if (!defined('WHMCS')) {
    die('This file cannot be accessed directly');
}

require_once __DIR__ . '/lib/Client.php';

use BreeferCP\Client;
use BreeferCP\ApiException;
use WHMCS\Database\Capsule;

function breefercp_MetaData()
{
    return [
        'DisplayName' => 'BreeferCP',
        'APIVersion' => '1.1',
        'RequiresServer' => true,
        'DefaultNonSSLPort' => '2087',
        'DefaultSSLPort' => '2087',
        'ServiceSingleSignOnLabel' => 'Log in to BreeferCP',
        'AdminSingleSignOnLabel' => 'Log in as customer',
    ];
}

function breefercp_ConfigOptions()
{
    return [
        'package_id' => [
            'FriendlyName' => 'Package ID',
            'Type' => 'text',
            'Size' => '34',
            'Description' => 'BreeferCP package id (Packages page in the panel)',
        ],
    ];
}

function breefercp_client(array $params): Client
{
    $host = $params['serverhostname'] ?: $params['serverip'];
    $port = $params['serverport'] ?: 2087;
    $scheme = !empty($params['serversecure']) ? 'https' : 'http';
    $token = $params['serveraccesshash'] ?: $params['serverpassword'];
    return new Client("{$scheme}://{$host}:{$port}", $token);
}

function breefercp_run(array $params, callable $fn)
{
    try {
        return $fn(breefercp_client($params));
    } catch (ApiException $e) {
        logModuleCall('breefercp', __FUNCTION__, [], $e->getMessage(), $e->getBody());
        return $e->getMessage();
    } catch (\Throwable $e) {
        logModuleCall('breefercp', __FUNCTION__, [], $e->getMessage(), '');
        return 'BreeferCP error: ' . $e->getMessage();
    }
}

function breefercp_CreateAccount(array $params)
{
    $result = breefercp_run($params, function (Client $c) use ($params) {
        $res = $c->post('/billing/accounts', [
            'username' => $params['username'],
            'email' => $params['clientsdetails']['email'],
            'domain' => $params['domain'],
            'package_id' => $params['configoption1'],
            'password' => $params['password'],
            'external_ref' => (string) $params['serviceid'],
        ]);
        $username = $res['account']['username'] ?? $params['username'];
        Capsule::table('tblhosting')->where('id', $params['serviceid'])->update(['username' => $username]);
        return 'success';
    });
    return $result;
}

function breefercp_SuspendAccount(array $params)
{
    return breefercp_run($params, function (Client $c) use ($params) {
        $c->post('/billing/accounts/' . rawurlencode((string) $params['serviceid']) . '/suspend', [
            'reason' => $params['suspendreason'] ?: 'Suspended by billing',
        ]);
        return 'success';
    });
}

function breefercp_UnsuspendAccount(array $params)
{
    return breefercp_run($params, function (Client $c) use ($params) {
        $c->post('/billing/accounts/' . rawurlencode((string) $params['serviceid']) . '/unsuspend');
        return 'success';
    });
}

function breefercp_TerminateAccount(array $params)
{
    return breefercp_run($params, function (Client $c) use ($params) {
        $c->post('/billing/accounts/' . rawurlencode((string) $params['serviceid']) . '/terminate');
        return 'success';
    });
}

function breefercp_ChangePackage(array $params)
{
    return breefercp_run($params, function (Client $c) use ($params) {
        $c->post('/billing/accounts/' . rawurlencode((string) $params['serviceid']) . '/package', [
            'package_id' => $params['configoption1'],
        ]);
        return 'success';
    });
}

function breefercp_ChangePassword(array $params)
{
    return breefercp_run($params, function (Client $c) use ($params) {
        $c->post('/billing/accounts/' . rawurlencode((string) $params['serviceid']) . '/password', [
            'password' => $params['password'],
        ]);
        return 'success';
    });
}

function breefercp_TestConnection(array $params)
{
    try {
        breefercp_client($params)->get('/billing/packages');
        return ['success' => true, 'error' => ''];
    } catch (\Throwable $e) {
        return ['success' => false, 'error' => $e->getMessage()];
    }
}

/** Refresh disk/bandwidth usage shown in WHMCS. */
function breefercp_UsageUpdate(array $params)
{
    try {
        $client = breefercp_client($params);
        $rows = Capsule::table('tblhosting')
            ->where('server', $params['serverid'])
            ->whereIn('domainstatus', ['Active', 'Suspended'])
            ->get(['id']);
        foreach ($rows as $row) {
            try {
                $a = $client->get('/billing/accounts/' . rawurlencode((string) $row->id));
            } catch (ApiException $e) {
                continue; // service no longer exists on the panel
            }
            Capsule::table('tblhosting')->where('id', $row->id)->update([
                'diskusage' => (int) ($a['disk_used_mb'] ?? 0),
                'bwusage' => (int) ($a['bw_used_mb'] ?? 0),
                'lastupdate' => Capsule::raw('now()'),
            ]);
        }
    } catch (\Throwable $e) {
        logModuleCall('breefercp', __FUNCTION__, [], $e->getMessage(), '');
    }
}

function breefercp_sso_url(array $params): string
{
    $res = breefercp_client($params)->post('/billing/accounts/' . rawurlencode((string) $params['serviceid']) . '/sso');
    // BreeferCP 0.3+ returns the absolute cPanel URL (port 2083); customers
    // cannot sign in on the WHM port the API is reached on.
    if (!empty($res['url']) && preg_match('#^https?://#', $res['url'])) {
        return $res['url'];
    }
    $host = $params['serverhostname'] ?: $params['serverip'];
    $port = $params['serverport'] ?: 2087;
    $scheme = !empty($params['serversecure']) ? 'https' : 'http';
    // The token travels in the URL fragment, so it never reaches server logs.
    return "{$scheme}://{$host}:{$port}/sso#token=" . rawurlencode($res['token']);
}

function breefercp_ServiceSingleSignOn(array $params)
{
    try {
        return ['success' => true, 'redirectTo' => breefercp_sso_url($params)];
    } catch (\Throwable $e) {
        return ['success' => false, 'errorMsg' => $e->getMessage()];
    }
}

function breefercp_AdminSingleSignOn(array $params)
{
    return breefercp_ServiceSingleSignOn($params);
}

function breefercp_ClientArea(array $params)
{
    $host = htmlspecialchars($params['serverhostname'] ?: $params['serverip'], ENT_QUOTES);
    $user = htmlspecialchars($params['username'], ENT_QUOTES);
    // The "Log in to BreeferCP" button is rendered by WHMCS itself from
    // ServiceSingleSignOn, so this area only shows connection details.
    return '<p>Control panel: <strong>' . $host . '</strong> &middot; Username: <strong>' . $user . '</strong></p>';
}

<?php

namespace Paymenter\Extensions\Servers\BreeferCP;

use App\Classes\Extension\Server;
use App\Models\Service;
use Illuminate\Http\Client\PendingRequest;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Str;

/**
 * BreeferCP server extension for Paymenter.
 *
 * Install: copy this directory to <paymenter>/extensions/Servers/BreeferCP,
 * then enable it under Admin > Extensions and fill in the panel URL and a
 * BreeferCP API token that carries the "billing" scope.
 *
 * Every call is keyed on the Paymenter service id (sent as external_ref), so
 * retried jobs are idempotent on the BreeferCP side.
 */
class BreeferCP extends Server
{
    public function getConfig($values = []): array
    {
        return [
            [
                'name' => 'host',
                'label' => 'Panel URL',
                'type' => 'text',
                'default' => 'https://panel.example.com:2087',
                'description' => 'Base URL of BreeferCP, including the port.',
                'required' => true,
                'validation' => 'url',
            ],
            [
                'name' => 'token',
                'label' => 'API token',
                'type' => 'text',
                'description' => 'Token with the "billing" scope (Profile > API tokens). Restrict it to this server\'s IP.',
                'required' => true,
                'encrypted' => true,
            ],
        ];
    }

    public function getProductConfig($values = []): array
    {
        return [
            [
                'name' => 'package_id',
                'label' => 'BreeferCP package ID',
                'type' => 'text',
                'description' => 'Id of the package (Packages page in the panel) this product provisions.',
                'required' => true,
            ],
        ];
    }

    public function testConfig(): bool|string
    {
        try {
            $this->http()->get('/billing/packages')->throw();
            return true;
        } catch (\Throwable $e) {
            return $e->getMessage();
        }
    }

    public function createServer(Service $service, $settings, $properties)
    {
        $domain = $properties['domain'] ?? null;
        if (!$domain) {
            throw new \Exception('A domain is required to provision a BreeferCP account.');
        }
        // "u" + service id always satisfies the panel's username rules (3-16, a-z0-9).
        $username = $properties['username'] ?? ('u' . $service->id);

        $res = $this->http()->post('/billing/accounts', array_filter([
            'username' => strtolower($username),
            'email' => $service->user->email,
            'domain' => strtolower($domain),
            'package_id' => $settings['package_id'],
            'external_ref' => (string) $service->id,
            'password' => $properties['password'] ?? null,
        ]))->throw()->json();

        foreach (['breefercp_username' => $res['account']['username'] ?? $username, 'breefercp_domain' => $domain] as $key => $value) {
            $service->properties()->updateOrCreate(['key' => $key], ['name' => Str::headline($key), 'value' => $value]);
        }
        if (!empty($res['password'])) {
            $service->properties()->updateOrCreate(
                ['key' => 'breefercp_password'],
                ['name' => 'Panel password', 'value' => $res['password']]
            );
        }

        return $res;
    }

    public function suspendServer(Service $service, $settings, $properties)
    {
        return $this->http()->post($this->path($service, 'suspend'), ['reason' => 'Suspended by billing'])->throw()->json() ?? true;
    }

    public function unsuspendServer(Service $service, $settings, $properties)
    {
        return $this->http()->post($this->path($service, 'unsuspend'))->throw()->json() ?? true;
    }

    public function terminateServer(Service $service, $settings, $properties)
    {
        return $this->http()->post($this->path($service, 'terminate'))->throw()->json() ?? true;
    }

    public function upgradeServer(Service $service, $settings, $properties)
    {
        return $this->http()->post($this->path($service, 'package'), [
            'package_id' => $settings['package_id'],
        ])->throw()->json() ?? true;
    }

    /** One-time login URL for the customer, used by the "Open panel" action. */
    public function loginUrl(Service $service): string
    {
        $res = $this->http()->post($this->path($service, 'sso'))->throw()->json();
        return rtrim($this->config('host'), '/') . '/sso#token=' . rawurlencode($res['token']);
    }

    private function path(Service $service, string $action): string
    {
        return '/billing/accounts/' . rawurlencode((string) $service->id) . '/' . $action;
    }

    private function http(): PendingRequest
    {
        return Http::baseUrl(rtrim($this->config('host'), '/') . '/api/v1')
            ->withToken($this->config('token'))
            ->acceptJson()
            ->asJson()
            ->timeout(60)
            ->connectTimeout(10);
    }
}

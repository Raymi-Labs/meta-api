// Display routing only. Every destination still enforces its own server permissions.
export function getEnterpriseEntry(user, workspace) {
  const platform =
    Boolean(user?.id) &&
    (Number(user.role) >= 100 ||
      (Number(user.role) >= 10 && workspace.enabled));
  const contact = platform
    ? {
        state: 'platform',
        platform,
        to: '/console/enterprise-admin',
        label: '企业管理',
      }
    : { state: 'contact', platform };
  if (!user?.id || !workspace.enabled) return contact;
  if (workspace.loading) return { state: 'loading', platform };
  if (workspace.error) return { state: 'error', platform };
  if (!workspace.isMember || !workspace.data) return contact;
  const role = workspace.data.member?.role;
  const manager = role === 'owner' || role === 'admin';
  if (!manager && role !== 'member') return { state: 'error', platform };
  return {
    state: 'member',
    platform,
    member: true,
    role: role === 'owner' ? '所有者' : role === 'admin' ? '管理员' : '成员',
    name: workspace.data.enterprise?.name,
    frozen: workspace.data.enterprise?.status === 'frozen',
    to: manager ? '/enterprise/console' : '/console/enterprise',
    label: manager ? '进入工作台' : '我的额度',
  };
}

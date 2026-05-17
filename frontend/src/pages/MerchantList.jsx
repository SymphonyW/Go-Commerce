import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import EmptyState from '../components/EmptyState';
import LoadingState from '../components/LoadingState';
import PageHeader from '../components/PageHeader';
import SectionCard from '../components/SectionCard';
import { merchantAPI } from '../services/api';
import { formatDateTime } from '../utils/display';

const MerchantList = () => {
  const [merchants, setMerchants] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const role = localStorage.getItem('role');
  const userId = Number(localStorage.getItem('user_id'));
  const canManageMerchants = role === 'merchant' || role === 'admin';

  useEffect(() => {
    const fetchMerchants = async () => {
      try {
        setLoading(true);
        const data = await merchantAPI.listMerchants();
        setMerchants(data.merchants || []);
      } catch (loadError) {
        setError(loadError.response?.data?.error || '获取商户列表失败');
      } finally {
        setLoading(false);
      }
    };

    fetchMerchants();
  }, []);

  return (
    <div className="merchant-list-page">
      <PageHeader
        eyebrow="Merchants"
        title="商户管理"
        subtitle="集中查看店铺信息，并按角色进入对应后台。"
        meta={`${merchants.length} 家`}
        actions={
          canManageMerchants && (
            <Link to="/merchants/create" className="btn btn-primary">
              创建新商户
            </Link>
          )
        }
      />

      {error && <div className="error-message">{error}</div>}
      {!canManageMerchants && <div className="info-message">当前账户没有商家写权限，仅可查看商户信息。</div>}

      <SectionCard title="商户列表" subtitle="演示商家也会在这里自然出现。">
        {loading ? (
          <LoadingState label="正在加载商户..." />
        ) : merchants.length === 0 ? (
          <EmptyState compact title="暂无商户" description="创建第一家店铺后，这里会开始沉淀店铺信息。" />
        ) : (
          <div className="merchants-grid">
            {merchants.map((merchant) => (
              <article key={merchant.id} className="merchant-card">
                <div>
                  <h3>{merchant.name}</h3>
                  <div className="merchant-card-meta">
                    <p>商户 ID：{merchant.id}</p>
                    <p>联系方式：{merchant.contact_info}</p>
                    <p>创建时间：{formatDateTime(merchant.created_at)}</p>
                  </div>
                </div>
                {(role === 'admin' || (role === 'merchant' && merchant.owner_user_id === userId)) && (
                  <div className="inline-actions">
                    <Link to={`/merchant?merchant_id=${merchant.id}`} className="btn btn-secondary btn-sm">
                      进入后台
                    </Link>
                    <Link to={`/merchant/products?merchant_id=${merchant.id}`} className="btn btn-secondary btn-sm">
                      管理商品
                    </Link>
                  </div>
                )}
              </article>
            ))}
          </div>
        )}
      </SectionCard>
    </div>
  );
};

export default MerchantList;
